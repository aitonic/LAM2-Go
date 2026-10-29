package usage

import (
	"context"
	"lam.local/lam2/internal/fault"
	"lam.local/lam2/internal/model"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestRawCallSelectedContextAndMissingSource(t *testing.T) {
	ctx := context.Background()
	home := t.TempDir()
	dir := filepath.Join(home, "sessions")
	if err := os.MkdirAll(dir, 0700); err != nil {
		t.Fatal(err)
	}
	path := filepath.Join(dir, "rollout-session.jsonl")
	var b strings.Builder
	for i := 1; i <= 24; i++ {
		b.WriteString("{\"entry\":")
		b.WriteString(string(rune('0' + i/10)))
		b.WriteString(string(rune('0' + i%10)))
		b.WriteString("}\n")
	}
	if err := os.WriteFile(path, []byte(b.String()), 0600); err != nil {
		t.Fatal(err)
	}
	event := model.UsageEvent{SourceFile: path, LineNumber: 20}
	raw, err := ReadRawCall(ctx, event, []string{home})
	if err != nil {
		t.Fatal(err)
	}
	if raw.FromLine != 8 || raw.ToLine != 24 || len(raw.Lines) != 17 || raw.Truncated {
		t.Fatalf("wrong raw window %+v", raw)
	}
	if !strings.Contains(raw.Lines[12], "20") {
		t.Fatalf("selected call missing from window: %+v", raw)
	}
	bad := model.UsageEvent{SourceFile: filepath.Join(dir, "missing.jsonl"), LineNumber: 1}
	if _, err := ReadRawCall(ctx, bad, []string{home}); fault.Code(err) != "RAW_SOURCE_MISSING" {
		t.Fatal("missing source not handled", err)
	}
	outside := model.UsageEvent{SourceFile: filepath.Join(t.TempDir(), "secrets.jsonl"), LineNumber: 1}
	if _, err := ReadRawCall(ctx, outside, []string{home}); err == nil {
		t.Fatal("arbitrary raw file accepted")
	}
}
