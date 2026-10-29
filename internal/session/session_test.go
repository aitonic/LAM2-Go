package session

import (
	"context"
	"encoding/json"
	"fmt"
	"lam.local/lam2/internal/account"
	"lam.local/lam2/internal/fault"
	"lam.local/lam2/internal/model"
	"lam.local/lam2/internal/storage"
	"os"
	"path/filepath"
	"testing"
	"time"
)

func setup(t *testing.T) (*Service, model.Profile) {
	t.Helper()
	st, err := storage.Open(t.TempDir())
	if err != nil {
		t.Fatal(err)
	}
	a := account.Service{Store: st}
	p, err := a.Create(context.Background(), "A", "profile", 1)
	if err != nil {
		t.Fatal(err)
	}
	return &Service{Store: st}, p
}
func writeSession(t *testing.T, p model.Profile, id string, at time.Time) string {
	t.Helper()
	path := filepath.Join(p.Home, "sessions", "rollout-"+id+".jsonl")
	b := fmt.Sprintf("{\"type\":\"session_meta\",\"payload\":{\"id\":%q,\"cwd\":\"/work/test\",\"model_provider\":\"openai\"}}\n{\"type\":\"response_item\",\"payload\":{\"type\":\"message\",\"role\":\"user\",\"content\":[{\"type\":\"input_text\",\"text\":\"Investigate parser\"}]}}\n", id)
	if err := os.WriteFile(path, []byte(b), 0600); err != nil {
		t.Fatal(err)
	}
	os.Chtimes(path, at, at)
	return path
}
func TestQueryPaginationAndTitle(t *testing.T) {
	s, p := setup(t)
	for i := 0; i < 25; i++ {
		writeSession(t, p, fmt.Sprintf("session-%02d", i), time.Now().Add(-time.Duration(i)*time.Hour))
	}
	page, err := s.Query(context.Background(), Query{ProfileID: p.ID, Search: "parser", Limit: 10, Offset: 10, Sort: "modified", Descending: true})
	if err != nil || page.Total != 25 || len(page.Items) != 10 {
		t.Fatal(page, err)
	}
	if page.Items[0].ID != "session-10" || page.Items[0].Title != "Investigate parser" {
		t.Fatal(page.Items[0])
	}
}
func TestProtectedDeletionAndArchiveRestore(t *testing.T) {
	s, p := setup(t)
	now := time.Now()
	for i := 0; i < 22; i++ {
		writeSession(t, p, fmt.Sprintf("session-%02d", i), now.Add(-time.Duration(i+10)*24*time.Hour))
	}
	plan, err := s.PlanDelete(context.Background(), []Ref{{p.ID, "session-00"}, {p.ID, "session-21"}})
	if err != nil {
		t.Fatal(err)
	}
	if len(plan.Blocked) != 1 || len(plan.Items) != 1 {
		t.Fatal(plan)
	}
	if err = s.Delete(context.Background(), plan.ID, false); fault.Code(err) != "CONFIRMATION_REQUIRED" {
		t.Fatal(err)
	}
	if err = s.Delete(context.Background(), plan.ID, true); err != nil {
		t.Fatal(err)
	}
	page, _ := s.Query(context.Background(), Query{Limit: 50})
	if page.Total != 21 {
		t.Fatal(page.Total)
	}
	if err = s.Restore(context.Background(), plan.ID, true); err != nil {
		t.Fatal(err)
	}
	page, _ = s.Query(context.Background(), Query{Limit: 50})
	if page.Total != 22 {
		t.Fatal(page.Total)
	}
}
func TestMalformedUTF8AndSymlinkAreContained(t *testing.T) {
	s, p := setup(t)
	path := writeSession(t, p, "good", time.Now())
	f, _ := os.OpenFile(path, os.O_APPEND|os.O_WRONLY, 0600)
	f.Write([]byte{0xff, '\n'})
	f.Close()
	outside := filepath.Join(t.TempDir(), "secret.jsonl")
	os.WriteFile(outside, []byte(`{"type":"session_meta","payload":{"id":"evil"}}`), 0600)
	os.Symlink(outside, filepath.Join(p.Home, "sessions", "evil.jsonl"))
	page, err := s.Query(context.Background(), Query{Limit: 50})
	if err != nil || page.Total != 1 {
		t.Fatal(page, err)
	}
	detail, err := s.Read(context.Background(), p.ID, "good")
	if err != nil || detail.SkippedLines != 1 {
		t.Fatal(detail, err)
	}
}
func TestChangedDeletePlanIsRejected(t *testing.T) {
	s, p := setup(t)
	for i := 0; i < 22; i++ {
		writeSession(t, p, fmt.Sprintf("s-%02d", i), time.Now().Add(-time.Duration(i+10)*24*time.Hour))
	}
	plan, _ := s.PlanDelete(context.Background(), []Ref{{p.ID, "s-21"}})
	path := filepath.Join(p.Home, "sessions", "rollout-s-21.jsonl")
	f, _ := os.OpenFile(path, os.O_APPEND|os.O_WRONLY, 0600)
	f.WriteString("{}\n")
	f.Close()
	if err := s.Delete(context.Background(), plan.ID, true); err == nil {
		t.Fatal("stale plan applied")
	}
}

func TestFilteredDeletePlanBeyondFirstThousand(t *testing.T) {
	s, p := setup(t)
	now := time.Now()
	for i := 0; i < 1022; i++ {
		writeSession(t, p, fmt.Sprintf("bulk-%04d", i), now.Add(-time.Duration(i+10)*24*time.Hour))
	}
	refs, e := s.SelectFiltered(context.Background(), Query{ProfileID: p.ID}, true)
	if e != nil {
		t.Fatal(e)
	}
	if len(refs) != 1002 {
		t.Fatal(len(refs))
	}
	plan, e := s.PlanDelete(context.Background(), refs)
	if e != nil || len(plan.Items) != 1002 {
		t.Fatal(len(plan.Items), e)
	}
}

func TestInterruptedArchiveRestoresFilesAndIndex(t *testing.T) {
	ctx := context.Background()
	s, p := setup(t)
	for i := 0; i < 22; i++ {
		writeSession(t, p, fmt.Sprintf("recovery-%02d", i), time.Now().Add(-time.Duration(i+10)*24*time.Hour))
	}
	plan, e := s.PlanDelete(ctx, []Ref{{p.ID, "recovery-21"}})
	if e != nil {
		t.Fatal(e)
	}
	if e = s.Delete(ctx, plan.ID, true); e != nil {
		t.Fatal(e)
	}
	mp := filepath.Join(s.Store.Root, "archive", plan.ID, "manifest.json")
	raw, e := os.ReadFile(mp)
	if e != nil {
		t.Fatal(e)
	}
	var manifest archiveManifest
	if e = json.Unmarshal(raw, &manifest); e != nil {
		t.Fatal(e)
	}
	manifest.Stage = "backed_up"
	raw, _ = json.Marshal(manifest)
	os.WriteFile(mp, raw, 0600)
	recovered, e := s.RecoverArchives(ctx)
	if e != nil || len(recovered) != 1 {
		t.Fatal(recovered, e)
	}
	if _, e = s.Resolve(ctx, p.ID, "recovery-21"); e != nil {
		t.Fatal(e)
	}
	recovered, e = s.RecoverArchives(ctx)
	if e != nil || len(recovered) != 0 {
		t.Fatal("recovery not idempotent", recovered, e)
	}
}
