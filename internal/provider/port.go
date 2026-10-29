package provider

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"lam.local/lam2/internal/fault"
	"lam.local/lam2/internal/model"
	"lam.local/lam2/internal/storage"
	"net"
	"path/filepath"
	"time"
)

type PortPlan struct {
	ID, Fingerprint, Stage string
	Revision               uint64
	NewPort                int
	Bindings               map[string]model.Binding
	CreatedAt              time.Time
}

func portFingerprint(p PortPlan) string {
	p.Fingerprint = ""
	p.Stage = "planned"
	b, _ := json.Marshal(p)
	return storage.Hash(b)
}
func (s *Service) portPath(id string) string {
	return filepath.Join(s.Store.Root, "plans", "gateway-port-"+id+".json")
}
func (s *Service) savePortPlan(ctx context.Context, p PortPlan) error {
	b, _ := json.Marshal(p)
	return storage.AtomicWrite(ctx, s.portPath(p.ID), b, 0600)
}
func (s *Service) PlanPortChange(ctx context.Context, port int) (PortPlan, error) {
	var p PortPlan
	if port < 1 || port > 65535 {
		return p, fault.New("GATEWAY_PORT_INVALID", "Choose a TCP port between 1 and 65535")
	}
	st, e := s.Store.Read(ctx)
	if e != nil {
		return p, e
	}
	p = PortPlan{ID: storage.NewID(), Stage: "planned", Revision: st.Revision, NewPort: port, Bindings: map[string]model.Binding{}, CreatedAt: time.Now().UTC()}
	newURL := fmt.Sprintf("http://127.0.0.1:%d/v1", port)
	changed := st.Settings.GatewayPort != port
	for id, b := range st.Bindings {
		if b.RouteKind == "gateway" {
			p.Bindings[id] = b
			if b.GatewayURL != newURL {
				changed = true
			}
		}
	}
	if !changed {
		return p, fault.New("GATEWAY_PORT_UNCHANGED", "Gateway already uses the requested port")
	}
	p.Fingerprint = portFingerprint(p)
	return p, s.savePortPlan(ctx, p)
}
func (s *Service) CommitPortChange(ctx context.Context, id, confirmation string) (err error) {
	if !storage.ValidID(id) {
		return fault.New("PLAN_INVALID", "Invalid port plan")
	}
	unlock, e := storage.Lock(ctx, filepath.Join(s.Store.Root, "provider.lock"))
	if e != nil {
		return e
	}
	defer unlock()
	raw, e := storage.ReadFile(s.portPath(id), 4<<20)
	if e != nil {
		return e
	}
	var plan PortPlan
	if json.Unmarshal(raw, &plan) != nil || plan.ID != id || plan.Fingerprint != confirmation || portFingerprint(plan) != confirmation {
		return fault.New("PLAN_INVALID", "Gateway port confirmation changed")
	}
	if plan.Stage == "applied" {
		return nil
	}
	if plan.Stage != "planned" || time.Since(plan.CreatedAt) > 30*time.Minute {
		return fault.New("PLAN_STALE", "Gateway port preview expired")
	}
	st, e := s.Store.Read(ctx)
	if e != nil {
		return e
	}
	if st.Revision != plan.Revision {
		return fault.New("PLAN_STALE", "Profiles or settings changed")
	}
	if s.Config == nil && len(plan.Bindings) > 0 {
		return fault.New("CONFIG_UNAVAILABLE", "Config editor unavailable")
	}
	// Do not stop the old service when the target port is already occupied.
	ln, e := net.Listen("tcp4", fmt.Sprintf("127.0.0.1:%d", plan.NewPort))
	if e != nil {
		return fault.New("GATEWAY_PORT_BUSY", "Target port is occupied; no listener was stopped")
	}
	defer ln.Close()
	for id, b := range plan.Bindings {
		p, ok := st.Profiles[id]
		if !ok {
			return fault.New("PROFILE_NOT_FOUND", "Bound profile missing")
		}
		h, e := storage.FileHash(filepath.Join(p.Home, "config.toml"))
		if e != nil {
			return e
		}
		if h != b.ConfigAfterHash {
			return fault.New("CONFIG_CONFLICT", "Bound configuration changed after preview")
		}
	}
	if s.BeforePortChange == nil {
		return fault.New("GATEWAY_UNAVAILABLE", "Gateway lifecycle check is required before moving its port")
	}
	if e = s.BeforePortChange(ctx); e != nil {
		return e
	}
	records := []bindJournal{}
	updated := map[string]model.Binding{}
	committed := false
	defer func() {
		if committed {
			return
		}
		errs := []error{}
		for i := len(records) - 1; i >= 0; i-- {
			j := records[i]
			if e := s.rollback(context.Background(), j, st.Profiles[j.ProfileID]); e != nil {
				errs = append(errs, e)
			}
		}
		if len(errs) > 0 {
			err = fault.Wrap("CONFIG_RECOVERY_REQUIRED", "Port migration rollback needs review", errors.Join(errs...))
		}
	}()
	for profileID, b := range plan.Bindings {
		p := st.Profiles[profileID]
		before, e := storage.ReadFile(filepath.Join(p.Home, "config.toml"), 4<<20)
		if e != nil {
			return e
		}
		next := b
		next.Generation++
		next.GatewayURL = fmt.Sprintf("http://127.0.0.1:%d/v1", plan.NewPort)
		jid := storage.NewID()
		j := bindJournal{ID: jid, ProfileID: p.ID, BindingID: b.ID, ExpectedGeneration: next.Generation, Stage: "prepared", BeforeHash: b.ConfigAfterHash, Backup: filepath.Join(s.Store.Root, "config-backups", jid+".toml"), CreatedAt: time.Now().UTC()}
		if e = storage.AtomicWrite(ctx, j.Backup, before, 0600); e != nil {
			return e
		}
		if e = s.writeJournal(ctx, j); e != nil {
			return e
		}
		records = append(records, j)
		if e = s.Config.Apply(ctx, p, j.BeforeHash, s.configEdits(st.Providers[b.ProviderID], next)); e != nil {
			return e
		}
		j.AfterHash, e = storage.FileHash(filepath.Join(p.Home, "config.toml"))
		if e != nil {
			return e
		}
		j.Stage = "applied"
		records[len(records)-1] = j
		if e = s.writeJournal(ctx, j); e != nil {
			return e
		}
		next.ConfigAfterHash = j.AfterHash
		updated[p.ID] = next
	}
	_, e = s.Store.Update(ctx, st.Revision, func(v *model.State) error {
		for id, b := range updated {
			v.Bindings[id] = b
		}
		v.Settings.GatewayPort = plan.NewPort
		return nil
	})
	if e != nil {
		return e
	}
	committed = true
	for _, j := range records {
		j.Stage = "verified"
		if e = s.writeJournal(ctx, j); e != nil {
			return e
		}
	}
	plan.Stage = "applied"
	if e = s.savePortPlan(ctx, plan); e != nil {
		return e
	}
	if s.OnChanged != nil {
		for id := range updated {
			s.OnChanged(st.Profiles[id].Home)
		}
	}
	// Lazy-start at the new port on the next launch, like a saved port migration.
	// This return does not claim that an upstream connection was tested.
	return nil
}
