package account

import (
	"context"
	"encoding/json"
	"lam.local/lam2/internal/fault"
	"lam.local/lam2/internal/storage"
	"os"
	"path/filepath"
	"testing"
)

func TestAuxiliaryRelayProfileHasMetadataButNoCopiedAuthentication(t *testing.T) {
	s, st := fixture(t)
	ctx := context.Background()
	a, e := s.Create(ctx, "Source", "profile", 1)
	if e != nil {
		t.Fatal(e)
	}
	state, _ := st.Read(ctx)
	b, e := s.Create(ctx, "Runtime", "profile", state.Revision)
	if e != nil {
		t.Fatal(e)
	}
	os.WriteFile(filepath.Join(a.Home, "auth.json"), []byte(`{"OPENAI_API_KEY":"source-only"}`), 0600)
	os.WriteFile(filepath.Join(b.Home, "auth.json"), []byte(`{"OPENAI_API_KEY":"runtime-only"}`), 0600)
	p, e := s.PlanRelay(ctx, RelayRequest{RuntimeProfileID: b.ID, SourceProfileID: a.ID, ProviderPolicy: "keep_target"})
	if e != nil {
		t.Fatal(e)
	}
	if _, e = os.Stat(filepath.Join(st.Root, "profiles", p.ProfileID)); !os.IsNotExist(e) {
		t.Fatal("plan created profile directory")
	}
	r, e := s.CreateRelay(ctx, p.ID, p.Fingerprint)
	if e != nil {
		t.Fatal(e)
	}
	if r.Relay == nil || r.Relay.RuntimeProfileID != b.ID || r.Relay.SourceProfileID != a.ID || r.Relay.ProviderPolicy != "keep_target" {
		t.Fatal(r)
	}
	raw, e := storage.ReadFile(filepath.Join(r.Home, ".managed-by-lam2.json"), 1<<20)
	if e != nil {
		t.Fatal(e)
	}
	var marker map[string]any
	json.Unmarshal(raw, &marker)
	if marker["relay"] == nil {
		t.Fatal("relay ownership metadata missing")
	}
	for _, name := range []string{"auth.json", "auth-f.json", "config.toml"} {
		if _, e = os.Stat(filepath.Join(r.Home, name)); !os.IsNotExist(e) {
			t.Fatal("copied identity/config", name)
		}
	}
	twice, e := s.CreateRelay(ctx, p.ID, p.Fingerprint)
	if e != nil || twice.ID != r.ID {
		t.Fatal("not idempotent", e)
	}
}
func TestRelayPlanRejectsChangedStateAndWrongConfirmation(t *testing.T) {
	s, st := fixture(t)
	ctx := context.Background()
	a, _ := s.Create(ctx, "A", "profile", 1)
	state, _ := st.Read(ctx)
	b, _ := s.Create(ctx, "B", "profile", state.Revision)
	if _, e := s.PlanRelay(ctx, RelayRequest{RuntimeProfileID: "missing", SourceProfileID: a.ID}); fault.Code(e) != "PROFILE_NOT_FOUND" {
		t.Fatal(e)
	}
	p, e := s.PlanRelay(ctx, RelayRequest{Name: "Relay", RuntimeProfileID: b.ID, SourceProfileID: a.ID})
	if e != nil {
		t.Fatal(e)
	}
	if _, e = s.CreateRelay(ctx, p.ID, "wrong"); fault.Code(e) != "PLAN_INVALID" {
		t.Fatal(e)
	}
	state, _ = st.Read(ctx)
	s.Edit(ctx, a.ID, "changed", "", "", a.Proxy, state.Revision)
	if _, e = s.CreateRelay(ctx, p.ID, p.Fingerprint); fault.Code(e) != "PLAN_STALE" {
		t.Fatal(e)
	}
	if _, e = os.Stat(filepath.Join(st.Root, "profiles", p.ProfileID)); !os.IsNotExist(e) {
		t.Fatal("stale plan wrote directory")
	}
}
