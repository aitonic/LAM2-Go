package provider

import (
	"context"
	"encoding/json"
	"errors"
	"io/fs"
	"lam.local/lam2/internal/fault"
	"lam.local/lam2/internal/model"
	"lam.local/lam2/internal/storage"
	"os"
	"path/filepath"
	"time"
)

type BindPlan struct {
	ID, ProfileID, ProviderID, Model, ConfigHash, Fingerprint string
	Revision                                                  uint64
	CreatedAt                                                 time.Time
}
type bindJournal struct {
	ID, ProfileID, BindingID, Stage, BeforeHash, AfterHash, Backup string
	BeforeMissing                                                  bool
	ProviderID                                                     string
	ProviderRevision                                               uint64
	ExpectedGeneration                                             uint64
	NewTokenRef                                                    model.CredentialRef
	CatalogPath, CatalogHash                                       string
	CreatedAt                                                      time.Time
}

func (s *Service) PlanBind(ctx context.Context, profileID, providerID, modelID string) (BindPlan, error) {
	var plan BindPlan
	st, e := s.Store.Read(ctx)
	if e != nil {
		return plan, e
	}
	profile, ok := st.Profiles[profileID]
	if !ok {
		return plan, fault.New("PROFILE_NOT_FOUND", "Profile not found")
	}
	if profile.AuthMode == "pat" {
		return plan, fault.New("PAT_BINDING_UNSUPPORTED", "Bind providers to isolated profiles, not shared PAT homes")
	}
	p, ok := st.Providers[providerID]
	if !ok {
		return plan, fault.New("PROVIDER_NOT_FOUND", "Provider not found")
	}
	if !p.HasModel(modelID) {
		return plan, fault.New("MODEL_INVALID", "Choose a model from the provider catalog")
	}
	if e = Validate(p); e != nil {
		return plan, e
	}
	hash, e := storage.FileHash(filepath.Join(profile.Home, "config.toml"))
	if e != nil {
		return plan, e
	}
	plan = BindPlan{ID: storage.NewID(), ProfileID: profileID, ProviderID: providerID, Model: modelID, ConfigHash: hash, Revision: st.Revision, CreatedAt: time.Now().UTC()}
	raw, _ := json.Marshal(plan)
	plan.Fingerprint = storage.Hash(raw)
	raw, _ = json.Marshal(plan)
	e = storage.AtomicWrite(ctx, filepath.Join(s.Store.Root, "plans", "bind-"+plan.ID+".json"), raw, 0600)
	return plan, e
}
func (s *Service) CommitBind(ctx context.Context, id, confirmation string) (model.Binding, error) {
	var result model.Binding
	if !storage.ValidID(id) {
		return result, fault.New("PLAN_INVALID", "Invalid plan ID")
	}
	raw, e := storage.ReadFile(filepath.Join(s.Store.Root, "plans", "bind-"+id+".json"), 1<<20)
	if e != nil {
		return result, e
	}
	var plan BindPlan
	if json.Unmarshal(raw, &plan) != nil || confirmation != plan.Fingerprint || time.Since(plan.CreatedAt) > 30*time.Minute {
		return result, fault.New("PLAN_STALE", "Binding preview expired or confirmation changed")
	}
	unlock, e := storage.Lock(ctx, filepath.Join(s.Store.Root, "provider.lock"))
	if e != nil {
		return result, e
	}
	defer unlock()
	st, e := s.Store.Read(ctx)
	if e != nil {
		return result, e
	}
	p, ok := st.Profiles[plan.ProfileID]
	pr, ok2 := st.Providers[plan.ProviderID]
	hash, he := storage.FileHash(filepath.Join(p.Home, "config.toml"))
	if !ok || !ok2 || he != nil || st.Revision != plan.Revision || hash != plan.ConfigHash {
		return result, fault.New("PLAN_STALE", "Profile, provider or config changed")
	}
	if s.Config == nil {
		return result, fault.New("CONFIG_UNAVAILABLE", "Codex App Server is required for configuration edits")
	}
	if pr.AuthKind != "none" {
		if _, e = s.Credential(ctx, pr.Credential); e != nil {
			return result, e
		}
	}
	result = model.Binding{ID: "binding-" + storage.NewID(), ProfileID: p.ID, ProviderID: pr.ID, Model: plan.Model, Generation: 1, RouteKind: "direct", CreatedAt: time.Now().UTC(), ConfigBeforeHash: hash}
	old, had := st.Bindings[p.ID]
	if had {
		result.Generation = old.Generation + 1
	}
	baseURL := pr.BaseURL
	if pr.Protocol == "chat_completions" || pr.Codex.RouteViaGateway {
		if s.Gateway == nil {
			return result, fault.New("GATEWAY_UNAVAILABLE", "Gateway service is unavailable")
		}
		if s.Secrets == nil {
			return result, fault.New("SECRET_UNAVAILABLE", "Gateway credentials require the system store")
		}
		result.RouteKind = "gateway"
		result.TokenRef = model.CredentialRef{Kind: "keychain", Service: "lam2.gateway", Account: result.ID, Version: result.Generation}
		token := storage.NewID() + storage.NewID()
		if e = s.Secrets.Set(ctx, result.TokenRef, token); e != nil {
			return result, e
		}
		baseURL, e = s.Gateway.Ensure(ctx, result)
		if e != nil {
			_ = s.Secrets.Delete(context.Background(), result.TokenRef)
			return result, e
		}
		result.GatewayURL = baseURL
	}
	cleanupToken := true
	defer func() {
		if cleanupToken && result.TokenRef.Kind != "" {
			_ = s.Secrets.Delete(context.Background(), result.TokenRef)
		}
	}()
	catalog, e := model.ModelsCatalog(pr.Models)
	if e != nil {
		return result, fault.Wrap("MODEL_CATALOG_INVALID", "Cannot create Codex model catalog", e)
	}
	result.CatalogPath = filepath.Join(s.Store.Root, "catalogs", result.ID+".json")
	result.CatalogHash = storage.Hash(catalog)
	if e = storage.AtomicWrite(ctx, result.CatalogPath, catalog, 0600); e != nil {
		return result, e
	}
	cleanupCatalog := true
	defer func() {
		if cleanupCatalog {
			s.removeCatalog(result)
		}
	}()
	edits := s.configEdits(pr, result)
	j := bindJournal{ID: plan.ID, ProfileID: p.ID, BindingID: result.ID, Stage: "prepared", BeforeHash: hash, BeforeMissing: hash == storage.MissingHash, CreatedAt: time.Now().UTC()}
	j.CatalogPath = result.CatalogPath
	j.CatalogHash = result.CatalogHash
	j.Backup = filepath.Join(s.Store.Root, "config-backups", plan.ID+".toml")
	if !j.BeforeMissing {
		before, err := storage.ReadFile(filepath.Join(p.Home, "config.toml"), 4<<20)
		if err != nil {
			return result, err
		}
		if err = storage.AtomicWrite(ctx, j.Backup, before, 0600); err != nil {
			return result, err
		}
	}
	result.ConfigBackup = j.Backup
	if had {
		result.ConfigBeforeHash = old.ConfigBeforeHash
		result.ConfigBackup = old.ConfigBackup
	}
	if e = s.writeJournal(ctx, j); e != nil {
		return result, e
	}
	if e = s.Config.Apply(ctx, p, hash, edits); e != nil {
		j.Stage = "needs_review"
		_ = s.writeJournal(context.Background(), j)
		return result, e
	}
	j.AfterHash, e = storage.FileHash(filepath.Join(p.Home, "config.toml"))
	if e != nil {
		return result, e
	}
	j.Stage = "applied"
	result.ConfigAfterHash = j.AfterHash
	if e = s.writeJournal(ctx, j); e != nil {
		return result, e
	}
	_, e = s.Store.Update(ctx, st.Revision, func(v *model.State) error {
		v.Bindings[p.ID] = result
		pending := v.PendingBindings[:0]
		for _, link := range v.PendingBindings {
			if link.ProfileID != p.ID {
				pending = append(pending, link)
			}
		}
		v.PendingBindings = pending
		return nil
	})
	if e != nil {
		re := s.rollback(context.Background(), j, p)
		if re != nil {
			return result, re
		}
		return result, e
	}
	cleanupToken = false
	cleanupCatalog = false
	j.Stage = "verified"
	if e = s.writeJournal(ctx, j); e != nil {
		return result, e
	}
	cleanupToken = false
	if had && old.TokenRef.Kind != "" {
		_ = s.Secrets.Delete(context.Background(), old.TokenRef)
	}
	if had {
		s.removeCatalog(old)
	}
	if s.OnChanged != nil {
		s.OnChanged(p.Home)
	}
	return result, nil
}
func (s *Service) writeJournal(ctx context.Context, j bindJournal) error {
	raw, _ := json.MarshalIndent(j, "", "  ")
	return storage.AtomicWrite(ctx, filepath.Join(s.Store.Root, "binding-transactions", j.ID+".json"), raw, 0600)
}
func (s *Service) rollback(ctx context.Context, j bindJournal, p model.Profile) error {
	path := filepath.Join(p.Home, "config.toml")
	hash, e := storage.FileHash(path)
	if e != nil {
		return e
	}
	if hash == j.BeforeHash {
		j.Stage = "rolled_back"
		return s.writeJournal(ctx, j)
	}
	if j.AfterHash == "" || hash != j.AfterHash {
		j.Stage = "needs_review"
		_ = s.writeJournal(ctx, j)
		return fault.New("CONFIG_RECOVERY_CONFLICT", "Config changed after binding; manual recovery required")
	}
	if j.BeforeMissing {
		e = os.Remove(path)
	} else {
		b, err := storage.ReadFile(j.Backup, 4<<20)
		if err != nil {
			return err
		}
		if storage.Hash(b) != j.BeforeHash {
			return fault.New("BACKUP_CORRUPT", "Config backup changed")
		}
		e = storage.AtomicWrite(ctx, path, b, 0600)
	}
	if e != nil {
		return e
	}
	j.Stage = "rolled_back"
	return s.writeJournal(ctx, j)
}
func (s *Service) Detach(ctx context.Context, profileID string, confirm bool) error {
	return s.detach(ctx, profileID, confirm, nil)
}
func (s *Service) detach(ctx context.Context, profileID string, confirm bool, plan *DetachPlan) error {
	if !confirm {
		return fault.New("CONFIRMATION_REQUIRED", "Confirm restoring the original Codex configuration")
	}
	unlock, e := storage.Lock(ctx, filepath.Join(s.Store.Root, "provider.lock"))
	if e != nil {
		return e
	}
	defer unlock()
	st, e := s.Store.Read(ctx)
	if e != nil {
		return e
	}
	b, ok := st.Bindings[profileID]
	if !ok {
		return fault.New("BINDING_NOT_FOUND", "Profile is not bound")
	}
	if plan != nil && (st.Revision != plan.Revision || b.ID != plan.BindingID || b.Generation != plan.Generation || b.ConfigAfterHash != plan.ConfigHash) {
		return fault.New("PLAN_STALE", "Binding changed after preview")
	}
	p, ok := st.Profiles[profileID]
	if !ok {
		return fault.New("PROFILE_NOT_FOUND", "Profile not found")
	}
	path := filepath.Join(p.Home, "config.toml")
	hash, e := storage.FileHash(path)
	if e != nil {
		return e
	}
	if hash != b.ConfigAfterHash {
		return fault.New("CONFIG_CONFLICT", "Codex config was edited externally; no changes were made")
	}
	var original []byte
	if b.ConfigBeforeHash != storage.MissingHash {
		if !storage.Within(b.ConfigBackup, []string{filepath.Join(s.Store.Root, "config-backups")}) {
			return fault.New("PATH_INVALID", "Config backup path is invalid")
		}
		original, e = storage.ReadFile(b.ConfigBackup, 4<<20)
		if e != nil {
			return e
		}
		if storage.Hash(original) != b.ConfigBeforeHash {
			return fault.New("BACKUP_CORRUPT", "Config backup changed")
		}
	}
	_, e = s.Store.Commit(ctx, st.Revision, func(v *model.State) ([]storage.Change, error) {
		delete(v.Bindings, profileID)
		return []storage.Change{{Path: path, Before: hash, Data: original, Delete: b.ConfigBeforeHash == storage.MissingHash, Mode: 0600}}, nil
	})
	if e == nil && b.TokenRef.Kind != "" {
		_ = s.Secrets.Delete(context.Background(), b.TokenRef)
	}
	if e == nil {
		s.removeCatalog(b)
		if s.OnChanged != nil {
			s.OnChanged(p.Home)
		}
	}
	return e
}
func (s *Service) Recover(ctx context.Context) ([]string, error) {
	dir := filepath.Join(s.Store.Root, "binding-transactions")
	entries, e := os.ReadDir(dir)
	if errors.Is(e, fs.ErrNotExist) {
		return nil, nil
	}
	if e != nil {
		return nil, e
	}
	st, e := s.Store.Read(ctx)
	if e != nil {
		return nil, e
	}
	issues := []string{}
	for _, entry := range entries {
		if entry.IsDir() {
			continue
		}
		raw, err := storage.ReadFile(filepath.Join(dir, entry.Name()), 1<<20)
		if err != nil {
			return issues, err
		}
		var j bindJournal
		if json.Unmarshal(raw, &j) != nil {
			return issues, fault.New("JOURNAL_CORRUPT", "Invalid binding recovery record")
		}
		if j.Stage == "verified" || j.Stage == "rolled_back" {
			continue
		}
		p, ok := st.Profiles[j.ProfileID]
		if !ok {
			issues = append(issues, j.ID)
			continue
		}
		if b, ok := st.Bindings[p.ID]; ok && b.ID == j.BindingID && (j.ExpectedGeneration == 0 || b.Generation == j.ExpectedGeneration) && (j.ProviderRevision == 0 || st.Providers[j.ProviderID].Revision == j.ProviderRevision) {
			j.Stage = "verified"
			_ = s.writeJournal(ctx, j)
			continue
		}
		if err = s.rollback(ctx, j, p); err != nil {
			issues = append(issues, j.ID)
		} else {
			s.removeCatalog(model.Binding{CatalogPath: j.CatalogPath, CatalogHash: j.CatalogHash})
			if j.NewTokenRef.Kind != "" && s.Secrets != nil {
				_ = s.Secrets.Delete(ctx, j.NewTokenRef)
			}
		}
	}
	return issues, nil
}
func (s *Service) AuthToken(ctx context.Context, bindingID string) (string, error) {
	st, e := s.Store.Read(ctx)
	if e != nil {
		return "", e
	}
	for _, b := range st.Bindings {
		if b.ID != bindingID {
			continue
		}
		if b.RouteKind == "gateway" {
			return s.Credential(ctx, b.TokenRef)
		}
		p, ok := st.Providers[b.ProviderID]
		if !ok {
			return "", fault.New("PROVIDER_NOT_FOUND", "Bound provider missing")
		}
		return s.Credential(ctx, p.Credential)
	}
	return "", fault.New("BINDING_NOT_FOUND", "Binding not found")
}
