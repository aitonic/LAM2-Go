package session

import (
	"context"
	"fmt"
	"lam.local/lam2/internal/model"
	"lam.local/lam2/internal/storage"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"
)

func TestLegacyThreadNamesAndArchivedUsageDiscovery(t *testing.T) {
	home := t.TempDir()
	root := filepath.Join(home, "sessions")
	arch := filepath.Join(home, "archived_sessions")
	os.MkdirAll(root, 0700)
	os.MkdirAll(arch, 0700)
	os.WriteFile(filepath.Join(root, "a.jsonl"), []byte("{\"type\":\"session_meta\",\"payload\":{\"id\":\"a\"}}\n"), 0600)
	os.WriteFile(filepath.Join(arch, "b.jsonl"), []byte("{\"type\":\"session_meta\",\"payload\":{\"id\":\"b\"}}\n"), 0600)
	os.WriteFile(filepath.Join(home, "session_index.jsonl"), []byte("{\"id\":\"a\",\"thread_name\":\"old\"}\n{\"id\":\"a\",\"thread_name\":\"new title\"}\n"), 0600)
	st, _ := storage.Open(t.TempDir())
	s := Service{Store: st}
	p := model.Profile{ID: "p", Home: home}
	active, e := s.ListProfile(context.Background(), p)
	if e != nil || len(active) != 1 || active[0].Title != "new title" {
		t.Fatal(active, e)
	}
	all, e := s.ListUsageSources(context.Background(), p)
	if e != nil || len(all) != 2 {
		t.Fatal(all, e)
	}
	// A cached file must use the latest index title without requiring its JSONL to change.
	os.WriteFile(filepath.Join(home, "session_index.jsonl"), []byte("{\"id\":\"a\",\"thread_name\":\"renamed\"}\n"), 0600)
	active, e = s.ListProfile(context.Background(), p)
	if e != nil || active[0].Title != "renamed" {
		t.Fatal(active, e)
	}
	for _, v := range all {
		if !strings.HasPrefix(v.Path, home) {
			t.Fatal(v)
		}
	}
}

func TestLatestMetadataBeyondHeadAndAllFilteredSelection(t *testing.T) {
	s, p := setup(t)
	path := writeSession(t, p, "long", time.Now())
	f, _ := os.OpenFile(path, os.O_APPEND|os.O_WRONLY, 0600)
	for n := 0; n < 180000; n++ {
		f.WriteString("{\"type\":\"noop\"}\n")
	}
	f.WriteString("{\"type\":\"turn_context\",\"payload\":{\"model\":\"new-model\",\"cwd\":\"/new\",\"model_provider\":\"new-provider\"}}\n")
	f.Close()
	rows, e := s.ListProfile(context.Background(), p)
	if e != nil || len(rows) != 1 || rows[0].Model != "new-model" || rows[0].CWD != "/new" {
		t.Fatal(rows, e)
	}
	for i := 0; i < 24; i++ {
		writeSession(t, p, fmt.Sprintf("old-%02d", i), time.Now().Add(-time.Duration(i+10)*24*time.Hour))
	}
	selected, e := s.SelectFiltered(context.Background(), Query{ProfileID: p.ID, Search: "parser", Limit: 1}, true)
	if e != nil || len(selected) != 5 {
		t.Fatal(selected, e)
	}
	stat, e := s.StorageSummary(context.Background(), p.ID)
	if e != nil || stat.Sessions != 25 || stat.Deletable != 5 || stat.Bytes == 0 {
		t.Fatal(stat, e)
	}
}

func TestArchiveUpdatesAndRestoresSessionIndex(t *testing.T) {
	s, p := setup(t)
	ctx := context.Background()
	for i := 0; i < 22; i++ {
		writeSession(t, p, fmt.Sprintf("s-%02d", i), time.Now().Add(-time.Duration(i+10)*24*time.Hour))
	}
	index := filepath.Join(p.Home, "session_index.jsonl")
	body := []byte("{\"id\":\"s-21\",\"thread_name\":\"archived name\"}\n{\"id\":\"s-00\",\"thread_name\":\"keep\"}\n")
	os.WriteFile(index, body, 0600)
	plan, e := s.PlanDelete(ctx, []Ref{{p.ID, "s-21"}})
	if e != nil {
		t.Fatal(e)
	}
	if e = s.Delete(ctx, plan.ID, true); e != nil {
		t.Fatal(e)
	}
	b, _ := os.ReadFile(index)
	if strings.Contains(string(b), "s-21") {
		t.Fatal("deleted thread still in index")
	}
	f, _ := os.OpenFile(index, os.O_APPEND|os.O_WRONLY, 0600)
	f.WriteString("{\"id\":\"new\",\"thread_name\":\"created later\"}\n")
	f.Close()
	if e = s.Restore(ctx, plan.ID, true); e != nil {
		t.Fatal(e)
	}
	b, _ = os.ReadFile(index)
	if !strings.Contains(string(b), "archived name") || !strings.Contains(string(b), "created later") {
		t.Fatal(string(b))
	}
}
