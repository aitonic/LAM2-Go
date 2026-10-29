// Package provider manages LAM provider metadata and explicit configuration transactions.
package provider

import (
	"context"
	"encoding/json"
	"errors"
	"io"
	"lam.local/lam2/internal/fault"
	"lam.local/lam2/internal/model"
	lr "lam.local/lam2/internal/runtime"
	"lam.local/lam2/internal/storage"
	"net"
	"net/http"
	"net/url"
	"os"
	"os/exec"
	"path/filepath"
	"sort"
	"strings"
	"time"
)

type Edit struct {
	KeyPath       string `json:"keyPath"`
	Value         any    `json:"value"`
	MergeStrategy string `json:"mergeStrategy"`
}
type ConfigEditor interface {
	Apply(context.Context, model.Profile, string, []Edit) error
}
type GatewayReady interface {
	Ensure(context.Context, model.Binding) (string, error)
}
type Service struct {
	ProfileToken     func(context.Context, string) (string, error)
	BeforePortChange func(context.Context) error
	Store            *storage.Store
	Secrets          storage.Secrets
	Config           ConfigEditor
	Gateway          GatewayReady
	Executable       string
	HTTP             *http.Client
	OnChanged        func(string)
}

func Validate(p model.Provider) error {
	if p.ID != "" && (!storage.ValidID(p.ID) || p.ID == "openai" || p.ID == "ollama" || p.ID == "lmstudio") {
		return fault.New("PROVIDER_INVALID", "Invalid or reserved provider ID")
	}
	if strings.TrimSpace(p.Name) == "" || len(p.Name) > 128 {
		return fault.New("PROVIDER_INVALID", "Provider name is required")
	}
	if p.Protocol != "responses" && p.Protocol != "chat_completions" {
		return fault.New("PROVIDER_INVALID", "Unsupported provider protocol")
	}
	if err := ValidateURL(p.BaseURL); err != nil {
		return err
	}
	if p.UpstreamPath != "" {
		u, e := url.Parse(p.UpstreamPath)
		if e != nil || u.IsAbs() || u.Host != "" || u.RawQuery != "" || u.Fragment != "" || strings.Contains(p.UpstreamPath, "..") || strings.ContainsAny(p.UpstreamPath, "\r\n\x00") {
			return fault.New("URL_INVALID", "Adapter upstream path must be a relative API path")
		}
	}
	if len(p.Models) > 10000 {
		return fault.New("PROVIDER_INVALID", "Too many models")
	}
	seen := map[string]bool{}
	for _, m := range p.Models {
		if m.ID == "" || len(m.ID) > 256 || strings.ContainsAny(m.ID, "\r\n\x00") || seen[m.ID] {
			return fault.New("MODEL_INVALID", "Invalid or duplicate model")
		}
		seen[m.ID] = true
	}
	if p.DefaultModel != "" && !seen[p.DefaultModel] {
		return fault.New("MODEL_INVALID", "Default model is not in the catalog")
	}
	switch p.AuthKind {
	case "none", "bearer", "api_key":
	default:
		return fault.New("AUTH_INVALID", "Unsupported authentication kind")
	}
	if p.AuthKind == "api_key" && !safeHeaderName(p.AuthHeader) {
		return fault.New("AUTH_INVALID", "API key header is invalid")
	}
	headerNames := map[string]bool{}
	for k, v := range p.EnvHeaders {
		canonical := http.CanonicalHeaderKey(k)
		if !safeHeaderName(k) || !lr.EnvName(v) || headerNames[canonical] || p.AuthKind == "bearer" && canonical == "Authorization" || p.AuthKind == "api_key" && strings.EqualFold(k, p.AuthHeader) {
			return fault.New("HEADER_INVALID", "Invalid, duplicate or reserved environment header")
		}
		headerNames[canonical] = true
	}
	if err := validateQueries(p.QueryParams); err != nil {
		return err
	}
	caps := p.Capabilities
	for _, v := range []string{caps.Streaming, caps.FunctionTools, caps.Images, caps.Reasoning, caps.StructuredOutputs, caps.CustomTools, caps.Namespaces} {
		if v != "unknown" && v != "supported" && v != "unsupported" && v != "partial" {
			return fault.New("CAPABILITY_INVALID", "Capabilities must be explicit")
		}
	}
	return nil
}
func ValidateURL(raw string) error {
	u, e := url.Parse(raw)
	if e != nil || u.Hostname() == "" || u.User != nil || u.RawQuery != "" || u.Fragment != "" {
		return fault.New("URL_INVALID", "Use an absolute endpoint without credentials, query or fragment")
	}
	if u.Scheme != "https" {
		ip := net.ParseIP(u.Hostname())
		if u.Scheme != "http" || (u.Hostname() != "localhost" && (ip == nil || !ip.IsLoopback())) {
			return fault.New("URL_INVALID", "HTTPS is required except for local loopback providers")
		}
	}
	return nil
}
func Endpoint(base, path string) (string, error) {
	if e := ValidateURL(base); e != nil {
		return "", e
	}
	u, _ := url.Parse(base)
	u.Path = strings.TrimRight(u.Path, "/") + "/" + strings.TrimLeft(path, "/")
	return u.String(), nil
}
func (s *Service) List(ctx context.Context) ([]model.Provider, error) {
	st, e := s.Store.Read(ctx)
	if e != nil {
		return nil, e
	}
	out := make([]model.Provider, 0, len(st.Providers))
	for _, p := range st.Providers {
		out = append(out, p)
	}
	sort.Slice(out, func(i, j int) bool { return out[i].Name < out[j].Name })
	return out, nil
}
func (s *Service) Save(ctx context.Context, p model.Provider, newSecret string, expected uint64) (model.Provider, error) {
	unlock, lockErr := storage.Lock(ctx, filepath.Join(s.Store.Root, "provider.lock"))
	if lockErr != nil {
		return p, lockErr
	}
	defer unlock()
	if e := Validate(p); e != nil {
		return p, e
	}
	if len(newSecret) > 2400 || strings.ContainsAny(newSecret, "\r\n\x00") {
		return p, fault.New("SECRET_INVALID", "Secret is too large or contains control characters")
	}
	st, e := s.Store.Read(ctx)
	if e != nil {
		return p, e
	}
	if st.Revision != expected {
		return p, fault.New("REVISION_CONFLICT", "Refresh provider state before saving")
	}
	old, exists := st.Providers[p.ID]
	if exists && p.Revision != old.Revision {
		return p, fault.New("REVISION_CONFLICT", "Provider changed")
	}
	if !exists && p.ID == "" {
		p.ID = "provider-" + storage.NewID()
	}
	if exists {
		if err := validateAttachedUpdate(st, old, p, newSecret != ""); err != nil {
			return p, err
		}
		p.CreatedAt = old.CreatedAt
		if p.Credential.Kind == "" {
			p.Credential = old.Credential
		}
		p.Revision = old.Revision + 1
	} else {
		p.CreatedAt = time.Now().UTC()
		p.Revision = 1
	}
	p.UpdatedAt = time.Now().UTC()
	wrote := false
	if newSecret != "" {
		if s.Secrets == nil {
			return p, fault.New("SECRET_UNAVAILABLE", "System credential store is unavailable")
		}
		p.Credential = model.CredentialRef{Kind: "keychain", Service: "lam.remote-provider", Account: p.ID + "/" + storage.NewID(), Version: p.Revision}
		if e = s.Secrets.Set(ctx, p.Credential, newSecret); e != nil {
			return p, fault.Wrap("SECRET_UNAVAILABLE", "Cannot store provider credential", e)
		}
		wrote = true
	}
	committed, saveErr := s.savePreparedProvider(ctx, st, p, old)
	e = saveErr
	if e != nil {
		if wrote && !committed {
			_ = s.Secrets.Delete(context.Background(), p.Credential)
		}
		return p, e
	}
	if wrote && old.Credential.Kind == "keychain" {
		_ = s.Secrets.Delete(context.Background(), old.Credential)
	}
	if s.OnChanged != nil {
		for _, b := range st.Bindings {
			if b.ProviderID == p.ID {
				if profile, ok := st.Profiles[b.ProfileID]; ok {
					s.OnChanged(profile.Home)
				}
			}
		}
	}
	return p, nil
}
func (s *Service) Delete(ctx context.Context, id string, expected uint64, confirm bool) error {
	if !confirm {
		return fault.New("CONFIRMATION_REQUIRED", "Confirm provider deletion")
	}
	st, e := s.Store.Read(ctx)
	if e != nil {
		return e
	}
	p, ok := st.Providers[id]
	if !ok {
		return fault.New("PROVIDER_NOT_FOUND", "Provider not found")
	}
	_, e = s.Store.Update(ctx, expected, func(v *model.State) error {
		for _, b := range v.Bindings {
			if b.ProviderID == id {
				return fault.New("PROVIDER_IN_USE", "Detach all profiles first")
			}
		}
		delete(v.Providers, id)
		return nil
	})
	if e == nil && p.Credential.Kind == "keychain" {
		_ = s.Secrets.Delete(context.Background(), p.Credential)
	}
	return e
}
func (s *Service) Credential(ctx context.Context, ref model.CredentialRef) (string, error) {
	var value string
	var e error
	switch ref.Kind {
	case "keychain":
		if s.Secrets == nil {
			return "", fault.New("SECRET_UNAVAILABLE", "System credential store is unavailable")
		}
		value, e = s.Secrets.Get(ctx, ref)
	case "env":
		if !lr.EnvName(ref.EnvKey) {
			return "", fault.New("AUTH_INVALID", "Invalid credential environment name")
		}
		value = os.Getenv(ref.EnvKey)
	case "codex_profile":
		if !storage.ValidID(ref.ProfileID) || s.ProfileToken == nil {
			return "", fault.New("CODEX_API_KEY_MISSING", "Choose an existing API-key profile")
		}
		value, e = s.ProfileToken(ctx, ref.ProfileID)
	case "command", "auth_command":
		st, err := s.Store.Read(ctx)
		if err != nil {
			return "", err
		}
		ap, ok := st.Approvals[ref.ApprovalID]
		if !ok {
			return "", fault.New("AUTH_COMMAND_UNAPPROVED", "Approve the exact credential command first")
		}
		hash, err := storage.FileHash(ap.Executable)
		if err != nil || hash != ap.SHA256 {
			return "", fault.New("AUTH_COMMAND_CHANGED", "Credential executable changed")
		}
		call, cancel := context.WithTimeout(ctx, 10*time.Second)
		defer cancel()
		cmd := exec.CommandContext(call, ap.Executable, ap.Args...)
		limited := &limitedOutput{limit: 4096}
		cmd.Stdout = limited
		cmd.Stderr = io.Discard
		e = cmd.Run()
		value = strings.TrimSpace(limited.String())
		if limited.exceeded {
			e = errors.New("output limit")
		}
	default:
		return "", fault.New("AUTH_MISSING", "Credential source is not configured")
	}
	if e != nil || value == "" || len(value) > 4096 || strings.ContainsAny(value, "\r\n\x00") {
		return "", fault.Wrap("AUTH_MISSING", "Credential cannot be read or is invalid", e)
	}
	return value, nil
}

type limitedOutput struct {
	strings.Builder
	limit    int
	exceeded bool
}

func (w *limitedOutput) Write(p []byte) (int, error) {
	if w.Len()+len(p) > w.limit {
		w.exceeded = true
		return 0, errors.New("output limit")
	}
	return w.Builder.Write(p)
}
func (s *Service) ApproveCommand(ctx context.Context, path string, args []string, expected uint64, confirm bool) (model.CommandApproval, error) {
	var ap model.CommandApproval
	if !confirm {
		return ap, fault.New("CONFIRMATION_REQUIRED", "Explicitly approve executable and arguments")
	}
	if !filepath.IsAbs(path) {
		return ap, fault.New("AUTH_COMMAND_INVALID", "Executable must be an absolute path")
	}
	hash, e := storage.FileHash(path)
	if e != nil || hash == storage.MissingHash {
		return ap, fault.New("AUTH_COMMAND_INVALID", "Executable does not exist")
	}
	info, e := os.Stat(path)
	if e != nil || info.Mode().Perm()&0111 == 0 {
		return ap, fault.New("AUTH_COMMAND_INVALID", "File is not executable")
	}
	ap = model.CommandApproval{ID: storage.NewID(), Executable: path, Args: args, SHA256: hash, ApprovedAt: time.Now().UTC()}
	_, e = s.Store.Update(ctx, expected, func(v *model.State) error { v.Approvals[ap.ID] = ap; return nil })
	return ap, e
}
func (s *Service) Headers(ctx context.Context, p model.Provider) (http.Header, error) {
	h := make(http.Header)
	h.Set("Content-Type", "application/json")
	if p.AuthKind != "none" {
		v, e := s.Credential(ctx, p.Credential)
		if e != nil {
			return nil, e
		}
		if p.AuthKind == "api_key" {
			h.Set(p.AuthHeader, v)
		} else {
			h.Set("Authorization", "Bearer "+v)
		}
	}
	for name, env := range p.EnvHeaders {
		v := os.Getenv(env)
		if strings.ContainsAny(v, "\r\n\x00") {
			return nil, fault.New("HEADER_INVALID", "Header environment contains invalid characters")
		}
		if v != "" {
			h.Set(name, v)
		}
	}
	return h, nil
}
func (s *Service) Discover(ctx context.Context, p model.Provider) ([]model.ProviderModel, error) {
	return s.discover(ctx, p, 30*time.Second, 4<<20)
}
func (s *Service) discover(ctx context.Context, p model.Provider, timeout time.Duration, maxBytes int64) ([]model.ProviderModel, error) {
	if e := Validate(p); e != nil {
		return nil, e
	}
	endpoint, e := EndpointFor(p, "models")
	if e != nil {
		return nil, e
	}
	headers, e := s.Headers(ctx, p)
	if e != nil {
		return nil, e
	}
	ctx, cancel := context.WithTimeout(ctx, timeout)
	defer cancel()
	req, e := http.NewRequestWithContext(ctx, http.MethodGet, endpoint, nil)
	if e != nil {
		return nil, e
	}
	req.Header = headers
	client := http.Client{Timeout: timeout, CheckRedirect: func(*http.Request, []*http.Request) error { return http.ErrUseLastResponse }}
	if s.HTTP != nil {
		client.Transport = s.HTTP.Transport
	}
	resp, e := client.Do(req)
	if e != nil {
		return nil, fault.Wrap("PROVIDER_UNREACHABLE", "Provider could not be reached", e)
	}
	defer resp.Body.Close()
	if resp.StatusCode != 200 {
		return nil, fault.New("PROVIDER_HTTP_ERROR", "Provider model discovery returned a non-success status")
	}
	raw, e := io.ReadAll(io.LimitReader(resp.Body, maxBytes+1))
	if e != nil || int64(len(raw)) > maxBytes {
		return nil, fault.New("RESPONSE_INVALID", "Model catalog exceeds maximum size")
	}
	var payload struct {
		Data []model.ProviderModel `json:"data"`
	}
	if json.Unmarshal(raw, &payload) != nil || payload.Data == nil {
		return nil, fault.New("RESPONSE_INVALID", "Provider model catalog is not JSON")
	}
	out := []model.ProviderModel{}
	seen := map[string]bool{}
	for _, m := range payload.Data {
		if m.ID != "" && !seen[m.ID] {
			out = append(out, m)
			seen[m.ID] = true
		}
	}
	sort.Slice(out, func(i, j int) bool { return out[i].ID < out[j].ID })
	return out, nil
}

func validateQueries(q map[string]string) error {
	for k, v := range q {
		lower := strings.ToLower(k)
		if k == "" || len(k) > 128 || len(v) > 2048 || strings.ContainsAny(k+v, "\r\n\x00") {
			return fault.New("QUERY_INVALID", "Invalid provider query parameter")
		}
		for _, word := range []string{"api_key", "apikey", "token", "secret", "auth"} {
			if strings.Contains(lower, word) {
				return fault.New("QUERY_CREDENTIAL_FORBIDDEN", "Credentials must not appear in query parameters")
			}
		}
	}
	return nil
}
func EndpointFor(p model.Provider, path string) (string, error) {
	if e := validateQueries(p.QueryParams); e != nil {
		return "", e
	}
	raw, e := Endpoint(p.BaseURL, path)
	if e != nil {
		return "", e
	}
	u, _ := url.Parse(raw)
	q := u.Query()
	for k, v := range p.QueryParams {
		q.Set(k, v)
	}
	u.RawQuery = q.Encode()
	return u.String(), nil
}

// safeHeaderName implements HTTP token syntax and excludes transport-owned fields.
func safeHeaderName(name string) bool {
	if name == "" {
		return false
	}
	for _, r := range name {
		if r >= 'a' && r <= 'z' || r >= 'A' && r <= 'Z' || r >= '0' && r <= '9' || strings.ContainsRune("!#$%&'*+-.^_`|~", r) {
			continue
		}
		return false
	}
	switch strings.ToLower(name) {
	case "host", "connection", "proxy-connection", "keep-alive", "transfer-encoding", "te", "trailer", "upgrade", "content-length", "content-type", "proxy-authorization", "proxy-authenticate":
		return false
	}
	return true
}

// Attached providers must not silently diverge from their projected config.
func validateAttachedUpdate(st model.State, old, next model.Provider, rotate bool) error {
	for _, b := range st.Bindings {
		if b.ProviderID == old.ID && !next.HasModel(b.Model) {
			return fault.New("MODEL_IN_USE", "Switch bound accounts before removing a selected model")
		}
	}
	return nil
}
