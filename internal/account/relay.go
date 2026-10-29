package account

import (
	"context"
	"encoding/json"
	"path/filepath"
	"strings"
	"time"

	"lam.local/lam2/internal/fault"
	"lam.local/lam2/internal/model"
	"lam.local/lam2/internal/storage"
)

// A relay profile is an independent, initially unauthenticated Codex home.
// Runtime/source references are provenance, never permission to copy credentials.
// This preserves the original auxiliary create_relay API, not removed bulk Sync.
type RelayRequest struct {
	Name             string `json:"name"`
	RuntimeProfileID string `json:"runtimeProfileId"`
	SourceProfileID  string `json:"sourceProfileId"`
	ProviderPolicy   string `json:"providerPolicy"`
}
type RelayPlan struct {
	ID          string       `json:"id"`
	ProfileID   string       `json:"profileId"`
	Request     RelayRequest `json:"request"`
	Revision    uint64       `json:"revision"`
	CreatedAt   time.Time    `json:"createdAt"`
	Fingerprint string       `json:"fingerprint"`
}

func (s *Service) relayPlanPath(id string) (string, error) {
	if !storage.ValidID(id) {
		return "", fault.New("PLAN_INVALID", "Invalid relay plan")
	}
	return storage.SafeJoin(s.Store.Root, filepath.Join("plans", "relay-profile-"+id+".json"))
}
func (s *Service) PlanRelay(ctx context.Context, r RelayRequest) (RelayPlan, error) {
	var p RelayPlan
	st, e := s.Store.Read(ctx)
	if e != nil {
		return p, e
	}
	runtime, ok := st.Profiles[r.RuntimeProfileID]
	if !ok {
		return p, fault.New("PROFILE_NOT_FOUND", "Relay runtime profile missing")
	}
	source, ok := st.Profiles[r.SourceProfileID]
	if !ok {
		return p, fault.New("PROFILE_NOT_FOUND", "Relay source profile missing")
	}
	if r.Name == "" {
		r.Name = runtime.Name + "-relay-" + source.Name
	}
	r.Name = strings.TrimSpace(r.Name)
	if !validName(r.Name) || len(r.ProviderPolicy) > 256 || strings.ContainsAny(r.ProviderPolicy, "\x00\r\n") {
		return p, fault.New("INVALID_RELAY", "Invalid relay name or provider policy")
	}
	p = RelayPlan{ID: storage.NewID(), ProfileID: "profile-" + storage.NewID(), Request: r, Revision: st.Revision, CreatedAt: time.Now().UTC()}
	b, _ := json.Marshal(p)
	p.Fingerprint = storage.Hash(b)
	b, _ = json.MarshalIndent(p, "", "  ")
	path, e := s.relayPlanPath(p.ID)
	if e != nil {
		return p, e
	}
	return p, storage.AtomicWrite(ctx, path, b, 0600)
}
func (s *Service) CreateRelay(ctx context.Context, id, confirmation string) (model.Profile, error) {
	var empty model.Profile
	path, e := s.relayPlanPath(id)
	if e != nil {
		return empty, e
	}
	unlock, e := storage.Lock(ctx, filepath.Join(s.Store.Root, "relay-profile.lock"))
	if e != nil {
		return empty, e
	}
	defer unlock()
	b, e := storage.ReadFile(path, 1<<20)
	if e != nil {
		return empty, e
	}
	var p RelayPlan
	if json.Unmarshal(b, &p) != nil || p.ID != id || p.Fingerprint != confirmation {
		return empty, fault.New("PLAN_INVALID", "Relay confirmation does not match")
	}
	verified := p
	verified.Fingerprint = ""
	raw, _ := json.Marshal(verified)
	if storage.Hash(raw) != confirmation {
		return empty, fault.New("PLAN_INVALID", "Relay plan changed")
	}
	st, e := s.Store.Read(ctx)
	if e != nil {
		return empty, e
	}
	if existing, ok := st.Profiles[p.ProfileID]; ok {
		if existing.Managed && existing.Name == p.Request.Name && existing.Relay != nil && existing.Relay.RuntimeProfileID == p.Request.RuntimeProfileID && existing.Relay.SourceProfileID == p.Request.SourceProfileID && existing.Relay.ProviderPolicy == p.Request.ProviderPolicy {
			return existing, nil
		}
		return empty, fault.New("PROFILE_CONFLICT", "Reserved relay profile changed")
	}
	if st.Revision != p.Revision || time.Since(p.CreatedAt) > 30*time.Minute {
		return empty, fault.New("PLAN_STALE", "Create a new relay preview")
	}
	return s.create(ctx, p.ProfileID, p.Request.Name, "profile", p.Revision, &model.RelayIdentity{RuntimeProfileID: p.Request.RuntimeProfileID, SourceProfileID: p.Request.SourceProfileID, ProviderPolicy: p.Request.ProviderPolicy}, "")
}
