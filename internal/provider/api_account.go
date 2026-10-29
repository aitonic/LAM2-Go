package provider

import (
	"context"
	"encoding/json"
	"errors"
	"path/filepath"
	"strings"
	"time"

	"lam.local/lam2/internal/account"
	"lam.local/lam2/internal/fault"
	"lam.local/lam2/internal/model"
	"lam.local/lam2/internal/storage"
)

type APIAccountService struct {
	Providers *Service
	Accounts  *account.Service
}
type APIAccountConnection struct {
	Profile  model.Profile  `json:"profile"`
	Provider model.Provider `json:"provider"`
	Binding  model.Binding  `json:"binding"`
}
type APIAccountPlan struct {
	ID, Name, ProfileID, ProviderID, Model, Fingerprint, Stage string
	Revision, ProviderRevision                                 uint64
	CreatedAt                                                  time.Time
}

func (s APIAccountService) planPath(id string) (string, error) {
	if !storage.ValidID(id) {
		return "", fault.New("PLAN_INVALID", "Invalid API-account plan")
	}
	return storage.SafeJoin(s.Providers.Store.Root, filepath.Join("plans", "api-create-"+id+".json"))
}
func (s APIAccountService) savePlan(ctx context.Context, p APIAccountPlan) error {
	path, e := s.planPath(p.ID)
	if e != nil {
		return e
	}
	b, _ := json.MarshalIndent(p, "", "  ")
	return storage.AtomicWrite(ctx, path, b, 0600)
}
func (s APIAccountService) PlanCreate(ctx context.Context, name, providerID, modelID string) (APIAccountPlan, error) {
	var plan APIAccountPlan
	if strings.TrimSpace(name) == "" || len(name) > 256 || strings.ContainsAny(name, "\x00\r\n") {
		return plan, fault.New("INVALID_NAME", "Invalid account name")
	}
	st, e := s.Providers.Store.Read(ctx)
	if e != nil {
		return plan, e
	}
	p, ok := st.Providers[providerID]
	if !ok {
		return plan, fault.New("PROVIDER_NOT_FOUND", "Provider missing")
	}
	if !p.HasModel(modelID) {
		return plan, fault.New("MODEL_INVALID", "Choose an available model")
	}
	if e = Validate(p); e != nil {
		return plan, e
	}
	plan = APIAccountPlan{ID: storage.NewID(), Name: strings.TrimSpace(name), ProfileID: "profile-" + storage.NewID(), ProviderID: providerID, Model: modelID, Stage: "planned", Revision: st.Revision, ProviderRevision: p.Revision, CreatedAt: time.Now().UTC()}
	b, _ := json.Marshal(plan)
	plan.Fingerprint = storage.Hash(b)
	return plan, s.savePlan(ctx, plan)
}
func (s APIAccountService) ExecuteCreate(ctx context.Context, id, confirmation string) (APIAccountConnection, error) {
	var out APIAccountConnection
	path, e := s.planPath(id)
	if e != nil {
		return out, e
	}
	unlock, e := storage.Lock(ctx, filepath.Join(s.Providers.Store.Root, "api-account.lock"))
	if e != nil {
		return out, e
	}
	defer unlock()
	b, e := storage.ReadFile(path, 1<<20)
	if e != nil {
		return out, e
	}
	var p APIAccountPlan
	if json.Unmarshal(b, &p) != nil || p.ID != id || p.Fingerprint != confirmation {
		return out, fault.New("PLAN_INVALID", "API account confirmation does not match")
	}
	verified := p
	verified.Fingerprint = ""
	verified.Stage = "planned"
	check, _ := json.Marshal(verified)
	if storage.Hash(check) != confirmation {
		return out, fault.New("PLAN_INVALID", "API account plan was modified")
	}
	if p.Stage == "applied" {
		return s.Connection(ctx, p.ProfileID)
	}
	if p.Stage == "rolled_back" || time.Since(p.CreatedAt) > 30*time.Minute {
		return out, fault.New("PLAN_STALE", "Create a new API-account preview")
	}
	st, e := s.Providers.Store.Read(ctx)
	if e != nil {
		return out, e
	}
	provider, ok := st.Providers[p.ProviderID]
	if !ok || provider.Revision != p.ProviderRevision {
		return out, fault.New("PLAN_STALE", "Provider changed since preview")
	}
	if p.Stage == "planned" {
		if st.Revision != p.Revision {
			return out, fault.New("PLAN_STALE", "Account state changed")
		}
		p.Stage = "creating"
		if e = s.savePlan(ctx, p); e != nil {
			return out, e
		}
	}
	profile, exists := st.Profiles[p.ProfileID]
	if !exists {
		profile, e = s.Accounts.CreateWithID(ctx, p.ProfileID, p.Name, "profile", st.Revision)
		if e != nil {
			return out, e
		}
	} else if !profile.Managed || profile.Name != p.Name {
		return out, fault.New("PROFILE_CONFLICT", "Reserved API profile was modified")
	}
	rollback := func(cause error) (APIAccountConnection, error) {
		now, re := s.Providers.Store.Read(context.Background())
		if re == nil {
			if _, bound := now.Bindings[p.ProfileID]; bound {
				return out, fault.Wrap("API_ACCOUNT_RECOVERY_REQUIRED", "Binding exists; inspect before removing account", cause)
			}
			re = s.Accounts.Delete(context.Background(), p.ProfileID, now.Revision, true)
		}
		if re == nil {
			p.Stage = "rolled_back"
			re = s.savePlan(context.Background(), p)
		}
		return out, errors.Join(cause, re)
	}
	if binding, ok := st.Bindings[p.ProfileID]; ok && binding.ProviderID == p.ProviderID && binding.Model == p.Model {
		p.Stage = "applied"
		if e = s.savePlan(ctx, p); e != nil {
			return out, e
		}
		return s.Connection(ctx, p.ProfileID)
	}
	plan, e := s.Providers.PlanBind(ctx, profile.ID, p.ProviderID, p.Model)
	if e != nil {
		return rollback(e)
	}
	if _, e = s.Providers.CommitBind(ctx, plan.ID, plan.Fingerprint); e != nil {
		return rollback(e)
	}
	p.Stage = "applied"
	if e = s.savePlan(ctx, p); e != nil {
		return out, e
	}
	return s.Connection(ctx, p.ProfileID)
}
func (s APIAccountService) Connection(ctx context.Context, id string) (APIAccountConnection, error) {
	var out APIAccountConnection
	st, e := s.Providers.Store.Read(ctx)
	if e != nil {
		return out, e
	}
	p, ok := st.Profiles[id]
	b, bound := st.Bindings[id]
	if !ok || !bound {
		return out, fault.New("API_ACCOUNT_NOT_FOUND", "Profile has no API-provider binding")
	}
	v, ok := st.Providers[b.ProviderID]
	if !ok {
		return out, fault.New("PROVIDER_NOT_FOUND", "Bound provider missing")
	}
	return APIAccountConnection{p, v, b}, nil
}
func (s APIAccountService) PlanModelSwitch(ctx context.Context, id, modelID string) (BindPlan, error) {
	c, e := s.Connection(ctx, id)
	if e != nil {
		return BindPlan{}, e
	}
	return s.Providers.PlanBind(ctx, id, c.Provider.ID, modelID)
}
func (s APIAccountService) UpdateConnection(ctx context.Context, id string, next model.Provider, key string, revision uint64) (model.Provider, error) {
	c, e := s.Connection(ctx, id)
	if e != nil {
		return next, e
	}
	if next.ID != c.Provider.ID {
		return next, fault.New("PROVIDER_INVALID", "Use explicit rebind to choose another provider")
	}
	return s.Providers.Save(ctx, next, key, revision)
}
func (s APIAccountService) Delete(ctx context.Context, id string, confirmed bool) error {
	if !confirmed {
		return fault.New("CONFIRMATION_REQUIRED", "Confirm API account removal")
	}
	before, e := s.Connection(ctx, id)
	if e != nil {
		return e
	}
	if e := s.Providers.Detach(ctx, id, true); e != nil {
		return e
	}
	st, e := s.Providers.Store.Read(ctx)
	if e != nil {
		return e
	}
	err := s.Accounts.Delete(ctx, id, st.Revision, true)
	if err == nil {
		return nil
	}
	// Deletion can fail before the account commit (e.g. ownership changed).
	// Restore its binding rather than leave a usable account silently detached.
	now, readErr := s.Providers.Store.Read(context.Background())
	if readErr != nil {
		return errors.Join(err, readErr)
	}
	if _, exists := now.Profiles[id]; exists {
		if _, bound := now.Bindings[id]; !bound {
			plan, rebindErr := s.Providers.PlanBind(context.Background(), id, before.Provider.ID, before.Binding.Model)
			if rebindErr == nil {
				_, rebindErr = s.Providers.CommitBind(context.Background(), plan.ID, plan.Fingerprint)
			}
			return errors.Join(err, rebindErr)
		}
	}
	return err
}
