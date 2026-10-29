package usage

import (
	"context"
	"lam.local/lam2/internal/model"
	"os"
	"path/filepath"
	"testing"
)

func TestLegacyParserDiagnosticCategoriesPersistAcrossAppend(t *testing.T) {
	home := t.TempDir()
	sessions := filepath.Join(home, "sessions")
	if e := os.MkdirAll(sessions, 0700); e != nil {
		t.Fatal(e)
	}
	path := filepath.Join(sessions, "s.jsonl")
	lines := []byte("not json\n" + `{"type":"event_msg","timestamp":"2026-10-20T12:00:00Z"}` + "\n" + `{"type":"event_msg","timestamp":"2026-10-20T12:00:00Z","payload":{"type":"token_count"}}` + "\n")
	if e := os.WriteFile(path, lines, 0600); e != nil {
		t.Fatal(e)
	}
	repo := &memoryRepo{}
	ix := Indexer{Repo: repo, Parser: func() Parser { return fixtureParser{} }, LockRoot: t.TempDir()}
	p := model.Profile{ID: "p", Home: home}
	s := model.Session{ID: "s", Path: path}
	result, e := ix.Index(context.Background(), p, s, nil, nil)
	if e != nil || result.Diagnostics == 0 {
		t.Fatal(result, e)
	}
	cp := repo.cp[result.Key]
	for key, want := range map[string]int64{"invalid_json": 1, "missing_payload": 1, "missing_info": 1, "skipped_events": 1} {
		if cp.ParserDiagnostics[key] != want {
			t.Fatalf("%s: %#v", key, cp.ParserDiagnostics)
		}
	}
	if cp.SkippedEvents != 1 {
		t.Fatalf("incorrect skipped_event count %d", cp.SkippedEvents)
	}
	if e := os.WriteFile(path, append(lines, []byte("broken\n")...), 0600); e != nil {
		t.Fatal(e)
	}
	result, e = ix.Index(context.Background(), p, s, nil, nil)
	if e != nil {
		t.Fatal(e)
	}
	if repo.cp[result.Key].ParserDiagnostics["invalid_json"] != 2 {
		t.Fatal("incremental diagnostics lost", repo.cp[result.Key])
	}
}
