package handoff

import (
	"context"
	"lam.local/lam2/internal/account"
	"lam.local/lam2/internal/fault"
	"lam.local/lam2/internal/model"
	"lam.local/lam2/internal/session"
	"lam.local/lam2/internal/storage"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func setup(t *testing.T) (*Service, model.Profile, model.Profile, string) {
	t.Helper()
	st, _ := storage.Open(t.TempDir())
	a := account.Service{Store: st}
	p, _ := a.Create(context.Background(), "A", "profile", 1)
	s, _ := st.Read(context.Background())
	q, _ := a.Create(context.Background(), "B", "profile", s.Revision)
	path := filepath.Join(p.Home, "sessions", "rollout-original.jsonl")
	os.WriteFile(path, []byte("{\"type\":\"session_meta\",\"payload\":{\"id\":\"original\",\"cwd\":\"/work\"}}\n{\"type\":\"event_msg\",\"payload\":{\"type\":\"user_message\",\"message\":\"hello\"}}\n"), 0600)
	os.WriteFile(filepath.Join(p.Home, "auth.json"), []byte("source-secret"), 0600)
	os.WriteFile(filepath.Join(q.Home, "auth.json"), []byte("target-secret"), 0600)
	return &Service{Store: st, Sessions: &session.Service{Store: st}}, p, q, path
}
func TestHandoffCopiesOnlySession(t *testing.T) {
	s, a, b, src := setup(t)
	ctx := context.Background()
	plan, err := s.Plan(ctx, a.ID, "original", b.ID, "stop", false)
	if err != nil {
		t.Fatal(err)
	}
	result, err := s.Execute(ctx, plan.ID, plan.Fingerprint)
	if err != nil {
		t.Fatal(err)
	}
	raw, _ := os.ReadFile(result.Path)
	orig, _ := os.ReadFile(src)
	if string(raw) != string(orig) {
		t.Fatal("session changed")
	}
	auth, _ := os.ReadFile(filepath.Join(b.Home, "auth.json"))
	if string(auth) != "target-secret" {
		t.Fatal("auth copied")
	}
	plan, err = s.Plan(ctx, a.ID, "original", b.ID, "stop", false)
	if err != nil {
		t.Fatal(err)
	}
	result, err = s.Execute(ctx, plan.ID, plan.Fingerprint)
	if err != nil || !result.Idempotent {
		t.Fatal(result, err)
	}
}
func TestHandoffConflictAndSourceChange(t *testing.T) {
	s, a, b, src := setup(t)
	ctx := context.Background()
	os.WriteFile(filepath.Join(b.Home, "sessions", "rollout-original.jsonl"), []byte("different"), 0600)
	plan, _ := s.Plan(ctx, a.ID, "original", b.ID, "stop", false)
	if len(plan.Blocked) == 0 {
		t.Fatal("unreported conflict")
	}
	if _, err := s.Execute(ctx, plan.ID, plan.Fingerprint); fault.Code(err) != "HANDOFF_CONFLICT" {
		t.Fatal(err)
	}
	plan, _ = s.Plan(ctx, a.ID, "original", b.ID, "replace", false)
	os.WriteFile(src, []byte("changed"), 0600)
	if _, err := s.Execute(ctx, plan.ID, plan.Fingerprint); fault.Code(err) != "PLAN_STALE" {
		t.Fatal(err)
	}
}
func TestHandoffRejectsSameProfileAndSymlink(t *testing.T) {
	s, a, b, _ := setup(t)
	ctx := context.Background()
	if _, err := s.Plan(ctx, a.ID, "original", a.ID, "stop", false); err == nil {
		t.Fatal("same profile")
	}
	os.RemoveAll(filepath.Join(b.Home, "sessions"))
	os.Symlink(t.TempDir(), filepath.Join(b.Home, "sessions"))
	if _, err := s.Plan(ctx, a.ID, "original", b.ID, "stop", false); err == nil {
		t.Fatal("symlink accepted")
	}
}
func TestHandoffForkHasNewIdentity(t *testing.T) {
	s, a, b, _ := setup(t)
	ctx := context.Background()
	plan, err := s.Plan(ctx, a.ID, "original", b.ID, "fork", false)
	if err != nil {
		t.Fatal(err)
	}
	r, err := s.Execute(ctx, plan.ID, plan.Fingerprint)
	if err != nil {
		t.Fatal(err)
	}
	data, _ := os.ReadFile(r.Path)
	if r.SessionID == "original" || !strings.Contains(string(data), r.SessionID) {
		t.Fatal("fork identity unchanged")
	}
}
