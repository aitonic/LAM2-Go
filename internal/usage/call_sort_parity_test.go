package usage

import (
	"context"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"lam.local/lam2/internal/model"
)

func TestLegacyDurationGapAttentionSortsDoNotSilentlyFallBackToTime(t *testing.T) {
	for _, key := range []string{"duration", "gap", "attention"} {
		sql := (Filter{SortKey: key}).CallOrderSQL()
		if strings.HasPrefix(sql, "timestamp ") || !strings.Contains(sql, " DESC") {
			t.Fatalf("%q silently falls back to time: %s", key, sql)
		}
	}
}

func TestLegacyThreadLinkMetadataSurvivesAppendAndReindex(t *testing.T) {
	home := t.TempDir()
	path := filepath.Join(home, "sessions", "s.jsonl")
	if err := os.MkdirAll(filepath.Dir(path), 0700); err != nil {
		t.Fatal(err)
	}
	line := func(at string, input int) string {
		return `{"type":"event_msg","timestamp":"` + at + `","payload":{"type":"token_count","info":{"total_token_usage":{"input_tokens":` + itoa(input) + `,"output_tokens":2}}}}` + "\n"
	}
	initial := line("2026-10-20T12:00:00Z", 20)
	second := line("2026-10-20T12:01:00Z", 30)
	if err := os.WriteFile(path, []byte(initial), 0600); err != nil {
		t.Fatal(err)
	}
	repo := &memoryRepo{}
	ix := Indexer{Repo: repo, Parser: func() Parser { return fixtureParser{} }, LockRoot: t.TempDir()}
	p := model.Profile{ID: "p", Home: home}
	s := model.Session{ID: "s", Path: path}
	if _, err := ix.Index(context.Background(), p, s, nil, nil); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(path, []byte(initial+second), 0600); err != nil {
		t.Fatal(err)
	}
	if _, err := ix.Index(context.Background(), p, s, nil, nil); err != nil {
		t.Fatal(err)
	}
	if len(repo.events) != 2 {
		t.Fatalf("want 2 records got %d", len(repo.events))
	}
	firstID := ""
	for _, ev := range repo.events {
		if ev.ThreadCallIndex == 1 {
			firstID = ev.ID
			if ev.PreviousRecordID != "" {
				t.Fatal(ev)
			}
		}
	}
	if firstID == "" {
		t.Fatal("first call index missing")
	}
	found := false
	for _, ev := range repo.events {
		if ev.ThreadCallIndex == 2 && ev.PreviousRecordID == firstID {
			found = true
		}
	}
	if !found {
		t.Fatal("missing stable previous-call link on append")
	}
	// No changes after rereading the same file.
	if _, err := ix.Index(context.Background(), p, s, nil, nil); err != nil {
		t.Fatal(err)
	}
	if len(repo.events) != 2 {
		t.Fatal("idempotency lost")
	}
}

func itoa(v int) string {
	if v == 20 {
		return "20"
	}
	return "30"
}
