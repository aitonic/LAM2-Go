// Package account implements LAM profile ownership and explicit identity changes.
package account

import (
	"context"
	"encoding/base64"
	"encoding/json"
	"errors"
	"lam.local/lam2/internal/fault"
	"lam.local/lam2/internal/model"
	"lam.local/lam2/internal/storage"
	"net/url"
	"os"
	"path/filepath"
	"sort"
	"strings"
	"time"
	"unicode/utf8"
)

type Service struct {
	Store         *storage.Store
	Vault         *storage.Vault
	SharedHome    string
	OnAuthChanged func(string)
}

func (s *Service) List(ctx context.Context) ([]model.Profile, error) {
	st, err := s.Store.Read(ctx)
	if err != nil {
		return nil, err
	}
	out := make([]model.Profile, 0, len(st.Profiles))
	for _, p := range st.Profiles {
		out = append(out, p)
	}
	sort.Slice(out, func(i, j int) bool {
		if out[i].Name == out[j].Name {
			return out[i].ID < out[j].ID
		}
		return out[i].Name < out[j].Name
	})
	return out, nil
}
func validName(name string) bool {
	return strings.TrimSpace(name) != "" && utf8.ValidString(name) && len(name) <= 256 && !strings.ContainsAny(name, "\x00\r\n")
}
func (s *Service) Create(ctx context.Context, name, mode string, expected uint64, copyConfigFrom ...string) (model.Profile, error) {
	if len(copyConfigFrom) > 1 {
		return model.Profile{}, fault.New("INVALID_TEMPLATE", "Choose one configuration source")
	}
	source := ""
	if len(copyConfigFrom) == 1 {
		source = copyConfigFrom[0]
	}
	return s.create(ctx, "profile-"+storage.NewID(), name, mode, expected, nil, source)
}

// CreateWithID allows a durable API-account plan to reserve a stable identity.
func (s *Service) CreateWithID(ctx context.Context, id, name, mode string, expected uint64) (model.Profile, error) {
	return s.create(ctx, id, name, mode, expected, nil, "")
}
func (s *Service) create(ctx context.Context, id, name, mode string, expected uint64, relay *model.RelayIdentity, copyConfigFrom string) (model.Profile, error) {
	if !storage.ValidID(id) || id == "main" {
		return model.Profile{}, fault.New("INVALID_ID", "Invalid managed profile ID")
	}

	var p model.Profile
	if !validName(name) {
		return p, fault.New("INVALID_NAME", "Enter a nonempty profile name without control characters")
	}
	if mode != "profile" && mode != "pat" {
		return p, fault.New("INVALID_AUTH_MODE", "Choose profile or pat mode")
	}
	st, err := s.Store.Read(ctx)
	if err != nil {
		return p, err
	}
	if st.Revision != expected {
		return p, fault.New("REVISION_CONFLICT", "Profile state changed; refresh and retry")
	}
	if _, ok := st.Profiles[id]; ok {
		return p, fault.New("PROFILE_EXISTS", "Profile ID already exists")
	}
	if relay != nil {
		for _, ref := range []string{relay.RuntimeProfileID, relay.SourceProfileID} {
			if _, ok := st.Profiles[ref]; !ok {
				return p, fault.New("PROFILE_NOT_FOUND", "Relay source or runtime profile missing")
			}
		}
	}
	var template []byte
	sourcePath, sourceHash := "", storage.MissingHash
	if copyConfigFrom != "" {
		source, ok := st.Profiles[copyConfigFrom]
		if !ok {
			return p, fault.New("PROFILE_NOT_FOUND", "Configuration source profile missing")
		}
		if _, bound := st.Bindings[copyConfigFrom]; bound {
			return p, fault.New("PROFILE_BOUND", "Bind the new account explicitly instead of copying another account's binding credentials")
		}
		sourcePath = filepath.Join(source.Home, "config.toml")
		template, err = storage.ReadFile(sourcePath, 4<<20)
		if err != nil && !os.IsNotExist(err) {
			return p, err
		}
		if err == nil {
			sourceHash = storage.Hash(template)
		}
	}
	home := filepath.Join(s.Store.Root, "profiles", id)
	now := time.Now().UTC()
	p = model.Profile{Relay: relay, ID: id, Name: strings.TrimSpace(name), Home: home, Managed: true, AuthMode: mode, Proxy: model.Proxy{Mode: "inherit"}, CreatedAt: now, UpdatedAt: now, AuthStatus: "not_logged_in"}
	{ // Every identity, including PAT, keeps its own credential/session directory.
		if _, e := os.Lstat(home); !os.IsNotExist(e) {
			return p, fault.New("PROFILE_EXISTS", "Managed home already exists")
		}
		if err = storage.EnsurePrivateDir(home); err != nil {
			return p, err
		}
		for _, dir := range []string{"sessions", "cache", "log", "tmp", "rules", "skills", "memories"} {
			if err = storage.EnsurePrivateDir(filepath.Join(home, dir)); err != nil {
				return p, err
			}
		}
	}
	_, err = s.Store.Commit(ctx, expected, func(v *model.State) ([]storage.Change, error) {
		v.Profiles[id] = p
		marker, _ := json.Marshal(map[string]any{"manager": "lam2", "profileId": id, "schemaVersion": 1, "relay": relay})
		changes := []storage.Change{{Path: filepath.Join(home, ".managed-by-lam2.json"), Before: storage.MissingHash, Data: marker}}
		if sourcePath != "" {
			nowHash, e := storage.FileHash(sourcePath)
			if e != nil {
				return nil, e
			}
			if nowHash != sourceHash {
				return nil, fault.New("CONFIG_CONFLICT", "Template configuration changed during creation")
			}
			if sourceHash != storage.MissingHash {
				changes = append(changes, storage.Change{Path: filepath.Join(home, "config.toml"), Before: storage.MissingHash, Data: template, Mode: 0600})
			}
		}
		return changes, nil
	})
	if err != nil {
		for _, dir := range []string{"sessions", "cache", "log", "tmp", "rules", "skills", "memories"} {
			_ = os.Remove(filepath.Join(home, dir))
		}
		_ = os.Remove(home)
	}
	return p, err
}
func (s *Service) Register(ctx context.Context, home, name string, expected uint64) (model.Profile, error) {
	var p model.Profile
	if !validName(name) {
		return p, fault.New("INVALID_NAME", "Enter a valid display name")
	}
	abs, err := filepath.Abs(home)
	if err != nil {
		return p, err
	}
	if err = storage.RejectSymlinks(abs); err != nil {
		return p, err
	}
	info, err := os.Stat(abs)
	if err != nil || !info.IsDir() {
		return p, fault.New("PROFILE_HOME_MISSING", "The selected Codex home does not exist")
	}
	id := "external-" + storage.Hash([]byte(abs))[:16]
	if filepath.Clean(abs) == filepath.Clean(s.SharedHome) {
		id = "main"
	}
	p = model.Profile{ID: id, Name: name, Home: abs, Managed: false, AuthMode: "profile", Proxy: model.Proxy{Mode: "inherit"}, CreatedAt: time.Now().UTC(), UpdatedAt: time.Now().UTC(), AuthStatus: "unverified"}
	_, err = s.Store.Update(ctx, expected, func(v *model.State) error {
		if _, exists := v.Profiles[id]; exists {
			return fault.New("PROFILE_EXISTS", "This profile identity is already registered")
		}
		for _, existing := range v.Profiles {
			if existing.Home == abs {
				return fault.New("PROFILE_EXISTS", "This home is already registered")
			}
		}
		v.Profiles[p.ID] = p
		return nil
	})
	return p, err
}
func Discover(userHome string) ([]model.Profile, error) {
	entries, err := os.ReadDir(userHome)
	if err != nil {
		return nil, err
	}
	out := []model.Profile{}
	for _, entry := range entries {
		if !entry.IsDir() || !(entry.Name() == ".codex" || strings.HasPrefix(entry.Name(), ".codex-")) {
			continue
		}
		path := filepath.Join(userHome, entry.Name())
		if storage.RejectSymlinks(path) != nil || !hasCodexSignal(path) {
			continue
		}
		id := "external-" + storage.Hash([]byte(path))[:16]
		name := strings.TrimPrefix(entry.Name(), ".")
		if entry.Name() == ".codex" {
			id, name = "main", "main"
		}
		out = append(out, model.Profile{ID: id, Name: name, Home: path, AuthMode: "profile", Managed: false, Proxy: model.Proxy{Mode: "inherit"}, AuthStatus: "unverified"})
	}
	return out, nil
}

// hasCodexSignal mirrors the original list_accounts discovery boundary. An
// empty directory named .codex-foo must not appear as an account.
func hasCodexSignal(home string) bool {
	for _, name := range []string{"auth.json", "config.toml", "history.jsonl", "sessions", "logs_2.sqlite", ".managed-by-agent-workspace.json", ".managed-by-codex-session-manager.json"} {
		if _, err := os.Lstat(filepath.Join(home, name)); err == nil {
			return true
		}
	}
	return false
}

// ListVisible immediately includes existing user Codex homes without silently
// importing, moving or mutating their credentials. Explicit registration is
// necessary before a discovered external profile is modified.
func (s *Service) ListVisible(ctx context.Context, userHome string) ([]model.Profile, error) {
	registered, err := s.List(ctx)
	if err != nil {
		return nil, err
	}
	discovered, err := Discover(userHome)
	if err != nil {
		return nil, err
	}
	seen := make(map[string]bool, len(registered))
	for _, p := range registered {
		seen[filepath.Clean(p.Home)] = true
	}
	for _, p := range discovered {
		if !seen[filepath.Clean(p.Home)] {
			registered = append(registered, p)
			seen[filepath.Clean(p.Home)] = true
		}
	}
	sort.Slice(registered, func(i, j int) bool {
		if registered[i].Name == registered[j].Name {
			return registered[i].ID < registered[j].ID
		}
		return registered[i].Name < registered[j].Name
	})
	return registered, nil
}

func (s *Service) Get(ctx context.Context, id string) (model.Profile, error) {
	st, err := s.Store.Read(ctx)
	if err != nil {
		return model.Profile{}, err
	}
	p, ok := st.Profiles[id]
	if !ok {
		return p, fault.New("PROFILE_NOT_FOUND", "The requested profile is not registered")
	}
	return p, nil
}
func (s *Service) Edit(ctx context.Context, id, name, note, renewal string, proxy model.Proxy, expected uint64) error {
	if !validName(name) || len(note) > 8192 {
		return fault.New("INVALID_PROFILE", "Invalid display name or note")
	}
	if renewal != "" {
		if _, err := time.Parse("2006-01-02", renewal); err != nil {
			return fault.New("INVALID_DATE", "Renewal date must be YYYY-MM-DD")
		}
	}
	if err := ValidateProxy(proxy); err != nil {
		return err
	}
	_, err := s.Store.Update(ctx, expected, func(v *model.State) error {
		p, ok := v.Profiles[id]
		if !ok {
			return fault.New("PROFILE_NOT_FOUND", "Profile not found")
		}
		p.Name = name
		p.Note = note
		p.RenewalDate = renewal
		p.Proxy = proxy
		p.UpdatedAt = time.Now().UTC()
		v.Profiles[id] = p
		return nil
	})
	if err == nil && s.OnAuthChanged != nil {
		p, _ := s.Get(ctx, id)
		s.OnAuthChanged(p.Home)
	}
	return err
}
func ValidateProxy(p model.Proxy) error {
	if p.Mode != "inherit" && p.Mode != "disabled" && p.Mode != "manual" {
		return fault.New("INVALID_PROXY", "Invalid proxy mode")
	}
	if p.Mode != "manual" {
		return nil
	}
	if p.HTTP == "" && p.HTTPS == "" && p.All == "" {
		return fault.New("INVALID_PROXY", "Manual proxy mode requires an address")
	}
	for _, raw := range []string{p.HTTP, p.HTTPS, p.All} {
		if raw == "" {
			continue
		}
		u, err := url.Parse(raw)
		if err != nil || u.Hostname() == "" || u.User != nil || u.RawQuery != "" || u.Fragment != "" || !(u.Scheme == "http" || u.Scheme == "https" || u.Scheme == "socks5" || u.Scheme == "socks5h") {
			return fault.New("INVALID_PROXY", "Use an HTTP or SOCKS proxy without embedded credentials")
		}
	}
	if strings.ContainsAny(p.NoProxy, "\r\n\x00") {
		return fault.New("INVALID_PROXY", "Invalid proxy bypass list")
	}
	return nil
}

// NormalizeAuth accepts existing Codex auth.json, CPA JSON or a browser Session JSON.
// It never fabricates an ID token, signs a JWT or infers quota from a credential.
func NormalizeAuth(raw []byte) ([]byte, string, error) {
	if len(raw) > 4<<20 || !utf8.Valid(raw) {
		return nil, "", fault.New("AUTH_IMPORT_INVALID", "Invalid credential JSON encoding or size")
	}
	var root map[string]any
	if json.Unmarshal(raw, &root) != nil || root == nil {
		return nil, "", fault.New("AUTH_IMPORT_INVALID", "Expected a credential JSON object")
	}
	email, _ := root["email"].(string)
	if u, ok := root["user"].(map[string]any); ok && email == "" {
		email, _ = u["email"].(string)
	}
	if pat, ok := root["personal_access_token"].(string); ok && strings.TrimSpace(pat) != "" {
		if strings.ContainsAny(pat, "\r\n\x00") {
			return nil, "", fault.New("AUTH_IMPORT_INVALID", "Invalid PAT value")
		}
		root["OPENAI_API_KEY"] = nil
		b, _ := json.Marshal(root)
		return b, email, nil
	}
	if key, ok := root["OPENAI_API_KEY"].(string); ok && strings.TrimSpace(key) != "" {
		if strings.ContainsAny(key, "\r\n\x00") {
			return nil, "", fault.New("AUTH_IMPORT_INVALID", "Invalid API key")
		}
		if _, ok := root["auth_mode"]; !ok {
			root["auth_mode"] = "apikey"
		}
		b, _ := json.Marshal(root)
		return b, email, nil
	}
	tokens, native := root["tokens"].(map[string]any)
	if tokens == nil {
		tokens = map[string]any{}
		for _, k := range []string{"access_token", "refresh_token", "id_token", "account_id"} {
			if v, ok := root[k].(string); ok && v != "" {
				tokens[k] = v
			}
		}
		if a, ok := root["accessToken"].(string); ok {
			tokens["access_token"] = a
		}
		if a, ok := root["account"].(map[string]any); ok {
			if id, ok := a["id"].(string); ok {
				tokens["account_id"] = id
			}
		}
	}
	access, _ := tokens["access_token"].(string)
	if strings.TrimSpace(access) == "" || strings.ContainsAny(access, "\r\n\x00") {
		return nil, "", fault.New("AUTH_IMPORT_INVALID", "An access token or API key is required")
	}
	clean := map[string]any{}
	for _, k := range []string{"access_token", "refresh_token", "id_token", "account_id"} {
		if v, ok := tokens[k].(string); ok && v != "" {
			if strings.ContainsAny(v, "\r\n\x00") {
				return nil, "", fault.New("AUTH_IMPORT_INVALID", "Credential fields contain control characters")
			}
			clean[k] = v
		}
	}
	if email == "" {
		if id, ok := clean["id_token"].(string); ok {
			email = jwtEmail(id)
		}
	}
	if native {
		// A native auth.json is already the source of truth. Validate known
		// secrets but retain future fields and the actual last refresh time.
		b, e := json.MarshalIndent(root, "", "  ")
		return b, email, e
	}
	result := map[string]any{"auth_mode": "chatgpt", "tokens": clean, "last_refresh": time.Now().UTC().Format(time.RFC3339)}
	if refresh, ok := root["last_refresh"]; ok {
		result["last_refresh"] = refresh
	}
	for _, k := range []string{"email", "expired", "plan_type", "chatgpt_plan_type", "headers", "websockets"} {
		if v, ok := root[k]; ok {
			result[k] = v
		}
	}
	b, _ := json.MarshalIndent(result, "", "  ")
	return b, email, nil
}
func jwtEmail(token string) string {
	parts := strings.Split(token, ".")
	if len(parts) != 3 {
		return ""
	}
	b, err := base64.RawURLEncoding.DecodeString(parts[1])
	if err != nil {
		return ""
	}
	var v struct {
		Email string `json:"email"`
	}
	_ = json.Unmarshal(b, &v)
	return v.Email
}
func (s *Service) ImportAuth(ctx context.Context, id string, raw []byte, confirmed bool) error {
	if !confirmed {
		return fault.New("CONFIRMATION_REQUIRED", "Importing authentication changes the selected profile")
	}
	p, err := s.Get(ctx, id)
	if err != nil {
		return err
	}
	if err = storage.RejectSymlinks(p.Home); err != nil {
		return err
	}
	if s.Vault == nil {
		return fault.New("KEYCHAIN_UNAVAILABLE", "An encrypted credential vault is required")
	}
	normal, quotaAuth, email, expiration, err := normalizeImport(raw)
	if err != nil {
		return err
	}
	defer clear(normal)
	defer clear(quotaAuth)
	unlock, err := storage.Lock(ctx, filepath.Join(s.Store.Root, "auth-"+id+".lock"))
	if err != nil {
		return err
	}
	defer unlock()
	st, err := s.Store.Read(ctx)
	if err != nil {
		return err
	}
	p, ok := st.Profiles[id]
	if !ok {
		return fault.New("PROFILE_NOT_FOUND", "Profile was removed")
	}
	credentialID := "auth-" + storage.NewID()
	if err = s.Vault.Put(ctx, credentialID, normal); err != nil {
		return err
	}
	quotaID := ""
	if len(quotaAuth) > 0 {
		quotaID = "quota-" + storage.NewID()
		if err = s.Vault.Put(ctx, quotaID, quotaAuth); err != nil {
			_ = s.Vault.Delete(context.Background(), credentialID)
			return err
		}
	}
	committed := false
	defer func() {
		if !committed {
			_ = s.Vault.Delete(context.Background(), credentialID)
			if quotaID != "" {
				_ = s.Vault.Delete(context.Background(), quotaID)
			}
		}
	}()
	var changes []storage.Change
	if p.Home != s.SharedHome {
		for name, data := range map[string][]byte{"auth.json": normal, "auth-f.json": quotaAuth} {
			if len(data) == 0 {
				continue
			}
			path := filepath.Join(p.Home, name)
			before, e := storage.FileHash(path)
			if e != nil {
				return e
			}
			changes = append(changes, storage.Change{Path: path, Before: before, Data: data})
		}
	}
	oldCredential, oldQuota := p.CredentialID, p.QuotaCredentialID
	_, err = s.Store.Commit(ctx, st.Revision, func(v *model.State) ([]storage.Change, error) {
		p.CredentialID = credentialID
		if quotaID != "" {
			p.QuotaCredentialID = quotaID
		}
		if expiration != nil {
			p.TokenExpiration = expiration
		}
		p.AuthStatus = "imported_unverified"
		p.Email = email
		p.UpdatedAt = time.Now().UTC()
		v.Profiles[id] = p
		return changes, nil
	})
	if err != nil {
		return err
	}
	committed = true
	if oldCredential != "" {
		_ = s.Vault.Delete(context.Background(), oldCredential)
	}
	if quotaID != "" && oldQuota != "" {
		_ = s.Vault.Delete(context.Background(), oldQuota)
	}
	if s.OnAuthChanged != nil {
		s.OnAuthChanged(p.Home)
	}
	return nil
}
func (s *Service) ExportCPA(ctx context.Context, id string, confirmed bool) ([]byte, error) {
	if !confirmed {
		return nil, fault.New("CONFIRMATION_REQUIRED", "Export contains live credentials; choose a private destination")
	}
	p, err := s.Get(ctx, id)
	if err != nil {
		return nil, err
	}
	runtime, err := s.readCredential(ctx, p, false)
	if err != nil {
		return nil, err
	}
	defer clear(runtime)
	secondary, secondaryErr := s.readCredential(ctx, p, true)
	if secondaryErr != nil && !errors.Is(secondaryErr, os.ErrNotExist) {
		return nil, secondaryErr
	}
	defer clear(secondary)
	source := runtime
	if len(secondary) > 0 {
		source = secondary
	}
	var auth, main map[string]any
	if json.Unmarshal(source, &auth) != nil || json.Unmarshal(runtime, &main) != nil {
		return nil, fault.New("CPA_UNSUPPORTED", "Invalid source credential")
	}
	tokens, _ := auth["tokens"].(map[string]any)
	if tokens == nil {
		tokens = auth
	}
	access, _ := tokens["access_token"].(string)
	if access == "" {
		return nil, fault.New("CPA_UNSUPPORTED", "CPA requires OAuth/Session credentials; add the separate quota credential for a PAT")
	}
	out := map[string]any{"type": "codex", "websockets": true, "email": p.Email}
	for _, k := range []string{"access_token", "refresh_token", "id_token", "account_id"} {
		out[k] = tokens[k]
	}
	for _, k := range []string{"last_refresh", "email", "expired", "plan_type", "chatgpt_plan_type", "websockets", "type", "headers"} {
		if v, ok := auth[k]; ok {
			out[k] = v
		}
	}
	if pat, ok := main["personal_access_token"].(string); ok && pat != "" {
		headers, _ := out["headers"].(map[string]any)
		if headers == nil {
			headers = map[string]any{}
		}
		headers["authorization"] = "Bearer " + pat
		out["headers"] = headers
	}
	return json.MarshalIndent(out, "", "  ")
}
