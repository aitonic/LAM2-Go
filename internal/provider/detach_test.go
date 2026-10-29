package provider

import (
	"context"
	"lam.local/lam2/internal/fault"
	"lam.local/lam2/internal/model"
	"os"
	"path/filepath"
	"testing"
)

func TestDetachPreviewIsReadOnlyAndRejectsStaleConfirmation(t *testing.T) {
	ctx := context.Background()
	s, p := setup(t)
	s.Config = &editor{}
	original := []byte("# original\nmodel=\"old\"\n")
	path := filepath.Join(p.Home, "config.toml")
	os.WriteFile(path, original, 0600)
	st, _ := s.Store.Read(ctx)
	pr, e := s.Save(ctx, candidate(), "test-key", st.Revision)
	if e != nil {
		t.Fatal(e)
	}
	bind, e := s.PlanBind(ctx, p.ID, pr.ID, "model-a")
	if e != nil {
		t.Fatal(e)
	}
	if _, e = s.CommitBind(ctx, bind.ID, bind.Fingerprint); e != nil {
		t.Fatal(e)
	}
	before, _ := os.ReadFile(path)
	plan, e := s.PlanDetach(ctx, p.ID)
	if e != nil {
		t.Fatal(e)
	}
	after, _ := os.ReadFile(path)
	if string(before) != string(after) {
		t.Fatal("preview wrote config")
	}
	if e = s.CommitDetach(ctx, plan.ID, "bad"); fault.Code(e) != "PLAN_INVALID" {
		t.Fatal(e)
	}
	st, _ = s.Store.Read(ctx)
	s.Store.Update(ctx, st.Revision, func(v *model.State) error { v.Profiles[p.ID] = p; return nil })
	if e = s.CommitDetach(ctx, plan.ID, plan.Fingerprint); fault.Code(e) != "PLAN_STALE" {
		t.Fatal(e)
	}
	after, _ = os.ReadFile(path)
	if string(after) != string(before) {
		t.Fatal("stale confirmation changed config")
	}
	plan, e = s.PlanDetach(ctx, p.ID)
	if e != nil {
		t.Fatal(e)
	}
	if e = s.CommitDetach(ctx, plan.ID, plan.Fingerprint); e != nil {
		t.Fatal(e)
	}
	after, _ = os.ReadFile(path)
	if string(after) != string(original) {
		t.Fatal("did not restore original config")
	}
}
