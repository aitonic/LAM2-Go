// Package migration imports metadata into the new store. It never rewrites
// legacy files, imports executable approvals, or raw-copies a live SQLite WAL.
package migration

import (
	"context"
	"encoding/json"
	"errors"
	"lam.local/lam2/internal/fault"
	"lam.local/lam2/internal/model"
	"lam.local/lam2/internal/provider"
	"lam.local/lam2/internal/storage"
	"os"
	"path/filepath"
	"strings"
	"time"
)

type UsageImporter interface {
	Stage(context.Context, string, string) error
	Publish(context.Context, string) error
	RollbackImport(context.Context, string) error
}
type Service struct {
	Store *storage.Store
	Usage UsageImporter
}
type Source struct{ Path, Hash string }
type Plan struct {
	PendingBindings       []model.PendingBinding
	Settings              *model.Settings
	LegacySettings        map[string]json.RawMessage
	PATTimeline           []model.PATPeriod
	ID, Root, Fingerprint string
	DatabasePath          string
	Revision              uint64
	Providers             []model.Provider
	Profiles              []model.Profile
	Sources               []Source
	Warnings              []string
	CreatedAt             time.Time
}
type record struct {
	Plan            Plan
	Stage           string
	Before          model.State
	AppliedRevision uint64
}

func DecodeProviders(raw []byte) ([]model.Provider, []string, error) {
	var entries []json.RawMessage
	warnings := []string{}
	if len(strings.TrimSpace(string(raw))) == 0 {
		return nil, nil, fault.New("MIGRATION_INVALID", "Empty provider document")
	}
	if strings.HasPrefix(strings.TrimSpace(string(raw)), "[") {
		if json.Unmarshal(raw, &entries) != nil {
			return nil, nil, fault.New("MIGRATION_INVALID", "Malformed legacy providers")
		}
	} else {
		var env struct {
			SchemaVersion int               `json:"schemaVersion"`
			SchemaSnake   int               `json:"schema_version"`
			Providers     []json.RawMessage `json:"providers"`
		}
		if json.Unmarshal(raw, &env) != nil {
			return nil, nil, fault.New("MIGRATION_INVALID", "Malformed provider store")
		}
		v := env.SchemaVersion
		if v == 0 {
			v = env.SchemaSnake
		}
		if v != 1 {
			return nil, nil, fault.New("SCHEMA_UNSUPPORTED", "Unknown legacy provider schema")
		}
		entries = env.Providers
	}
	out := make([]model.Provider, 0, len(entries))
	seen := map[string]bool{}
	for _, entry := range entries {
		var old struct {
			ID, Name, Protocol, BaseURL, DefaultModel, WireAPI, EnvKey, SecretStorage string
			Models                                                                    []model.ProviderModel
			UpstreamAuth                                                              struct {
				Kind, Name string
				Source     model.CredentialRef
			}
			Capabilities model.Capabilities
			Codex        struct {
				DisplayName                                     string
				StreamIdleTimeoutMs                             *uint64
				DirectRequestMaxRetries, DirectStreamMaxRetries *uint8
				RouteViaGateway                                 bool
				QueryParams, EnvHttpHeaders                     map[string]string
				ReasoningEffort                                 string
			}
			Adapter struct {
				UpstreamPath string `json:"upstreamPath"`
			}
			CompatibilityProfile string
			CreatedAt, UpdatedAt string
		}
		// encoding/json matches field names case-insensitively, including baseUrl/wireApi.
		if json.Unmarshal(entry, &old) != nil {
			return nil, nil, fault.New("MIGRATION_INVALID", "Malformed provider entry")
		}
		p := model.Provider{ID: old.ID, Name: old.Name, Protocol: old.Protocol, BaseURL: old.BaseURL, DefaultModel: old.DefaultModel, Models: old.Models, Capabilities: old.Capabilities, AuthKind: old.UpstreamAuth.Kind, Credential: old.UpstreamAuth.Source, AuthHeader: old.UpstreamAuth.Name, EnvHeaders: old.Codex.EnvHttpHeaders, QueryParams: old.Codex.QueryParams, ReasoningEffort: old.Codex.ReasoningEffort, UpstreamPath: old.Adapter.UpstreamPath, Compatibility: old.CompatibilityProfile, Revision: 1, Codex: model.CodexOptions{DisplayName: old.Codex.DisplayName, StreamIdleTimeoutMS: old.Codex.StreamIdleTimeoutMs, RequestMaxRetries: old.Codex.DirectRequestMaxRetries, StreamMaxRetries: old.Codex.DirectStreamMaxRetries, RouteViaGateway: old.Codex.RouteViaGateway}}
		if p.Protocol == "" {
			switch old.WireAPI {
			case "openai":
				p.Protocol = "chat_completions"
			case "responses":
				p.Protocol = "responses"
			default:
				return nil, nil, fault.New("MIGRATION_PROTOCOL_UNSUPPORTED", "Unknown legacy wireApi")
			}
			if old.SecretStorage == "env" && old.EnvKey != "" {
				p.AuthKind = "bearer"
				p.Credential = model.CredentialRef{Kind: "env", EnvKey: old.EnvKey}
			} else {
				return nil, nil, fault.New("MIGRATION_CREDENTIAL_REQUIRED", "Legacy non-reference credentials require explicit encrypted import")
			}
		}
		if p.AuthKind == "header" {
			p.AuthKind = "api_key"
		}
		if p.AuthKind == "none" {
			p.Credential = model.CredentialRef{Kind: "none"}
		}
		if p.Credential.Kind == "auth_command" {
			warnings = append(warnings, p.ID+": executable approval must be reviewed in the new application")
		}
		if p.Credential.Kind == "codex_profile" {
			warnings = append(warnings, p.ID+": account credential reference requires manual remapping")
		}
		if len(p.Models) == 0 && p.DefaultModel != "" {
			p.Models = []model.ProviderModel{{ID: p.DefaultModel, Label: p.DefaultModel}}
		}
		fillCaps(&p.Capabilities)
		for j := range p.Models {
			if p.Models[j].Capabilities != nil {
				fillCaps(p.Models[j].Capabilities)
			}
		}
		if err := provider.Validate(p); err != nil {
			return nil, nil, err
		}
		if p.ID == "" || seen[p.ID] {
			return nil, nil, fault.New("MIGRATION_DUPLICATE", "Missing or duplicate provider ID")
		}
		seen[p.ID] = true
		p.CreatedAt, _ = time.Parse(time.RFC3339, old.CreatedAt)
		p.UpdatedAt, _ = time.Parse(time.RFC3339, old.UpdatedAt)
		out = append(out, p)
	}
	return out, warnings, nil
}
func fillCaps(c *model.Capabilities) {
	for _, v := range []*string{&c.Streaming, &c.FunctionTools, &c.StructuredOutputs, &c.Reasoning, &c.Images, &c.CustomTools, &c.Namespaces} {
		if *v == "" {
			*v = "unknown"
		}
	}
}
func fp(p Plan) string { p.Fingerprint = ""; raw, _ := json.Marshal(p); return storage.Hash(raw) }
func (s *Service) Plan(ctx context.Context, root string) (Plan, error) {
	var p Plan
	root, e := filepath.Abs(root)
	if e != nil {
		return p, e
	}
	if storage.Within(root, []string{s.Store.Root}) || storage.Within(s.Store.Root, []string{root}) {
		return p, fault.New("MIGRATION_ROOT_INVALID", "Legacy and destination roots must be separate")
	}
	if e = storage.RejectSymlinks(root); e != nil {
		return p, e
	}
	st, e := s.Store.Read(ctx)
	if e != nil {
		return p, e
	}
	p = Plan{ID: storage.NewID(), Root: root, Revision: st.Revision, CreatedAt: time.Now().UTC(), Providers: []model.Provider{}, Profiles: []model.Profile{}, Warnings: []string{"Bindings require explicit re-attach to the new executable. No legacy executable approval is copied.", "Legacy SQLite is imported logically through MyGo, including WAL and SQLite-only history; stop the legacy writer during the read."}}
	sources := []string{"provider-hub/providers.json", "config/accounts-cache.json"}
	for _, rel := range sources {
		path, e := storage.SafeJoin(root, rel)
		if e != nil {
			return p, e
		}
		raw, e := storage.ReadFile(path, 8<<20)
		if errors.Is(e, os.ErrNotExist) {
			continue
		}
		if e != nil {
			return p, e
		}
		p.Sources = append(p.Sources, Source{path, storage.Hash(raw)})
		if strings.HasSuffix(rel, "providers.json") {
			p.Providers, p.Warnings, e = decodeWithWarnings(raw, p.Warnings)
			if e != nil {
				return p, e
			}
			continue
		}
		var cache struct {
			Accounts []struct {
				ID, DisplayName, CodexHome, AuthMode, Note string
				RelayIdentity, RelaySource                 string
				IsRelay                                    bool
				RenewalDate                                *string
			}
		}
		if json.Unmarshal(raw, &cache) != nil {
			return p, fault.New("MIGRATION_INVALID", "Malformed legacy account cache")
		}
		homes := map[string]bool{}
		for _, a := range cache.Accounts {
			if !filepath.IsAbs(a.CodexHome) || a.ID == "" {
				p.Warnings = append(p.Warnings, "Ignored account cache row without absolute home or stable ID")
				continue
			}
			if e = storage.RejectSymlinks(a.CodexHome); e != nil {
				return p, e
			}
			info, e := os.Stat(a.CodexHome)
			if e != nil || !info.IsDir() {
				p.Warnings = append(p.Warnings, a.ID+": home is missing; account not imported")
				continue
			}
			if homes[a.CodexHome] {
				p.Warnings = append(p.Warnings, a.ID+": shared-home identity requires explicit PAT import")
				continue
			}
			homes[a.CodexHome] = true
			name := a.DisplayName
			if name == "" {
				name = a.ID
			}
			profile := model.Profile{ID: a.ID, Name: name, Home: a.CodexHome, AuthMode: "profile", Managed: false, Note: a.Note, Proxy: model.Proxy{Mode: "inherit"}, AuthStatus: "unverified", CreatedAt: time.Now().UTC(), UpdatedAt: time.Now().UTC()}
			if a.IsRelay || a.RelayIdentity != "" || a.RelaySource != "" {
				runtime, source := a.RelayIdentity, a.RelaySource
				if runtime == "" || source == "" {
					if left, right, ok := strings.Cut(a.ID, "-relay-"); ok {
						runtime, source = left, right
					}
				}
				profile.Relay = &model.RelayIdentity{RuntimeProfileID: runtime, SourceProfileID: source}
			}
			if a.RenewalDate != nil {
				profile.RenewalDate = *a.RenewalDate
			}
			p.Profiles = append(p.Profiles, profile)
		}
	}
	if e = s.bindingMetadata(&p); e != nil {
		return p, e
	}
	if e = s.metadata(ctx, &p, st.Settings); e != nil {
		return p, e
	}
	database := filepath.Join(root, "usage", "usage.sqlite3")
	if info, err := os.Stat(database); err == nil && info.Mode().IsRegular() {
		p.DatabasePath = database
		for _, path := range []string{database, database + "-wal"} {
			hash, err := storage.FileHash(path)
			if errors.Is(err, os.ErrNotExist) && path != database {
				hash = ""
			} else if err != nil {
				return p, err
			}
			p.Sources = append(p.Sources, Source{path, hash})
		}
	}
	if len(p.Sources) == 0 {
		p.Warnings = append(p.Warnings, "No supported legacy metadata found; existing Codex homes may be registered individually")
	}
	p.Fingerprint = fp(p)
	r := record{Plan: p, Stage: "planned", Before: st}
	e = s.save(ctx, r)
	return p, e
}
func decodeWithWarnings(b []byte, w []string) ([]model.Provider, []string, error) {
	p, more, e := DecodeProviders(b)
	return p, append(w, more...), e
}
func (s *Service) load(id string) (record, error) {
	var r record
	if !storage.ValidID(id) {
		return r, fault.New("PLAN_INVALID", "Invalid migration ID")
	}
	b, e := storage.ReadFile(filepath.Join(s.Store.Root, "migrations", id+".json"), 16<<20)
	if e != nil {
		return r, e
	}
	if json.Unmarshal(b, &r) != nil {
		return r, fault.New("MIGRATION_INVALID", "Corrupt migration manifest")
	}
	return r, nil
}
func (s *Service) save(ctx context.Context, r record) error {
	b, _ := json.MarshalIndent(r, "", "  ")
	return storage.AtomicWrite(ctx, filepath.Join(s.Store.Root, "migrations", r.Plan.ID+".json"), b, 0600)
}
func (s *Service) Apply(ctx context.Context, id, confirmation string) error {
	r, e := s.load(id)
	if e != nil {
		return e
	}
	p := r.Plan
	if p.Fingerprint != confirmation || fp(p) != confirmation {
		return fault.New("CONFIRMATION_REQUIRED", "Migration preview fingerprint does not match")
	}
	if r.Stage == "applied" {
		return nil
	}
	if r.Stage == "metadata_applied" {
		if p.DatabasePath != "" {
			if s.Usage == nil {
				return fault.New("DATABASE_UNAVAILABLE", "Usage importer is unavailable")
			}
			if e = s.Usage.Publish(ctx, id); e != nil {
				return e
			}
		}
		r.Stage = "applied"
		return s.save(ctx, r)
	}
	if time.Since(p.CreatedAt) > time.Hour || r.Stage != "planned" {
		return fault.New("PLAN_STALE", "Create a new migration preview")
	}
	for _, src := range p.Sources {
		hash, e := storage.FileHash(src.Path)
		if errors.Is(e, os.ErrNotExist) && src.Hash == "" {
			continue
		}
		if e != nil || hash != src.Hash {
			return fault.New("PLAN_STALE", "Legacy source changed; create a new preview")
		}
	}
	if p.DatabasePath != "" {
		if s.Usage == nil {
			return fault.New("DATABASE_UNAVAILABLE", "Usage importer is unavailable; no source history was skipped")
		}
		if e = s.Usage.Stage(ctx, p.DatabasePath, id); e != nil {
			return e
		}
	}
	// Commit metadata and its recovery marker together. Publication is retryable;
	// until it succeeds no caller receives a completed migration result.
	_, e = s.Store.Commit(ctx, p.Revision, func(st *model.State) ([]storage.Change, error) {
		for _, pr := range p.Providers {
			if _, ok := st.Providers[pr.ID]; ok {
				return nil, fault.New("MIGRATION_CONFLICT", "Provider ID already exists")
			}
			st.Providers[pr.ID] = pr
		}
		for _, a := range p.Profiles {
			if _, ok := st.Profiles[a.ID]; ok {
				return nil, fault.New("MIGRATION_CONFLICT", "Profile ID already exists")
			}
			for _, existing := range st.Profiles {
				if existing.Home == a.Home {
					return nil, fault.New("MIGRATION_CONFLICT", "Codex home is already registered")
				}
			}
			st.Profiles[a.ID] = a
		}
		if p.Settings != nil {
			st.Settings = *p.Settings
		}
		st.PATTimeline = append(st.PATTimeline, p.PATTimeline...)
		st.PendingBindings = append(st.PendingBindings, p.PendingBindings...)
		r.Stage = "metadata_applied"
		r.AppliedRevision = p.Revision + 1
		raw, _ := json.MarshalIndent(r, "", "  ")
		path := filepath.Join(s.Store.Root, "migrations", id+".json")
		hash, e := storage.FileHash(path)
		if e != nil {
			return nil, e
		}
		return []storage.Change{{Path: path, Before: hash, Data: raw}}, nil
	})
	if e != nil {
		return e
	}
	if p.DatabasePath != "" {
		if e = s.Usage.Publish(ctx, id); e != nil {
			return e
		}
	}
	r.Stage = "applied"
	return s.save(ctx, r)
}
func (s *Service) Rollback(ctx context.Context, id string, confirmed bool) error {
	if !confirmed {
		return fault.New("CONFIRMATION_REQUIRED", "Confirm metadata rollback")
	}
	r, e := s.load(id)
	if e != nil {
		return e
	}
	if r.Stage == "rollback_pending" {
		if r.Plan.DatabasePath != "" {
			if s.Usage == nil {
				return fault.New("DATABASE_UNAVAILABLE", "Usage rollback adapter unavailable")
			}
			if e = s.Usage.RollbackImport(ctx, id); e != nil {
				return e
			}
		}
		r.Stage = "rolled_back"
		return s.save(ctx, r)
	}
	if r.Stage != "applied" && r.Stage != "metadata_applied" {
		return fault.New("MIGRATION_STATE_INVALID", "Migration was not applied")
	}
	_, e = s.Store.Commit(ctx, r.AppliedRevision, func(st *model.State) ([]storage.Change, error) {
		rev := st.Revision
		*st = r.Before
		st.Revision = rev
		r.Stage = "rollback_pending"
		raw, _ := json.MarshalIndent(r, "", "  ")
		path := filepath.Join(s.Store.Root, "migrations", id+".json")
		hash, e := storage.FileHash(path)
		if e != nil {
			return nil, e
		}
		return []storage.Change{{Path: path, Before: hash, Data: raw}}, nil
	})
	if e != nil {
		return e
	}
	if r.Plan.DatabasePath != "" {
		if s.Usage == nil {
			return fault.New("DATABASE_UNAVAILABLE", "Usage rollback adapter unavailable")
		}
		if e = s.Usage.RollbackImport(ctx, id); e != nil {
			return e
		}
	}
	r.Stage = "rolled_back"
	return s.save(ctx, r)
}
