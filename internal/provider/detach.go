package provider

import (
	"context"
	"encoding/json"
	"path/filepath"
	"time"

	"lam.local/lam2/internal/fault"
	"lam.local/lam2/internal/storage"
)

type DetachPlan struct {
	ID          string    `json:"id"`
	ProfileID   string    `json:"profileId"`
	BindingID   string    `json:"bindingId"`
	Revision    uint64    `json:"revision"`
	Generation  uint64    `json:"generation"`
	ConfigHash  string    `json:"configHash"`
	CreatedAt   time.Time `json:"createdAt"`
	Fingerprint string    `json:"fingerprint"`
}

func (s *Service) detachPlanPath(id string) (string, error) {
	if !storage.ValidID(id) {
		return "", fault.New("PLAN_INVALID", "Invalid detach plan")
	}
	return storage.SafeJoin(s.Store.Root, filepath.Join("plans", "detach-"+id+".json"))
}
func (s *Service) PlanDetach(ctx context.Context, profileID string) (DetachPlan, error) {
	var p DetachPlan
	st, e := s.Store.Read(ctx)
	if e != nil {
		return p, e
	}
	b, ok := st.Bindings[profileID]
	if !ok {
		return p, fault.New("BINDING_NOT_FOUND", "Profile is not bound")
	}
	profile, ok := st.Profiles[profileID]
	if !ok {
		return p, fault.New("PROFILE_NOT_FOUND", "Profile not found")
	}
	hash, e := storage.FileHash(filepath.Join(profile.Home, "config.toml"))
	if e != nil {
		return p, e
	}
	if hash != b.ConfigAfterHash {
		return p, fault.New("CONFIG_CONFLICT", "Configuration was edited outside LAM")
	}
	p = DetachPlan{ID: storage.NewID(), ProfileID: profileID, BindingID: b.ID, Revision: st.Revision, Generation: b.Generation, ConfigHash: hash, CreatedAt: time.Now().UTC()}
	raw, _ := json.Marshal(p)
	p.Fingerprint = storage.Hash(raw)
	raw, _ = json.MarshalIndent(p, "", "  ")
	path, e := s.detachPlanPath(p.ID)
	if e != nil {
		return p, e
	}
	return p, storage.AtomicWrite(ctx, path, raw, 0600)
}
func (s *Service) CommitDetach(ctx context.Context, id, confirmation string) error {
	path, e := s.detachPlanPath(id)
	if e != nil {
		return e
	}
	raw, e := storage.ReadFile(path, 1<<20)
	if e != nil {
		return e
	}
	var p DetachPlan
	if json.Unmarshal(raw, &p) != nil || p.ID != id || p.Fingerprint != confirmation {
		return fault.New("PLAN_INVALID", "Detach confirmation does not match")
	}
	verified := p
	verified.Fingerprint = ""
	raw, _ = json.Marshal(verified)
	if storage.Hash(raw) != confirmation {
		return fault.New("PLAN_INVALID", "Detach plan was modified")
	}
	if time.Since(p.CreatedAt) > 30*time.Minute {
		return fault.New("PLAN_STALE", "Create a new detach preview")
	}
	return s.detach(ctx, p.ProfileID, true, &p)
}
