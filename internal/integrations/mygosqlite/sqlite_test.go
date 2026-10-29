package mygosqlite

import (
	"context"
	"fmt"
	"github.com/egoist/mygo/plugins/sqlite"
	"lam.local/lam2/internal/model"
	"lam.local/lam2/internal/usage"
	"path/filepath"
	"strings"
	"testing"
	"time"
)

func TestCheckpointAndEventsAreAtomic(t *testing.T) {
	ctx := context.Background()
	db, e := Open(ctx, filepath.Join(t.TempDir(), "usage.sqlite3"))
	if e != nil {
		t.Fatal(e)
	}
	defer db.Close()
	cp := usage.Checkpoint{Key: "k", Path: "/session", Offset: 100}
	event := model.UsageEvent{ID: "r", ProfileID: "p", SourceFile: "/session", SessionID: "s", Model: "m", Timestamp: time.Now(), Tokens: model.Tokens{Input: 100, CachedInput: 40, Output: 20}}
	if e = db.Commit(ctx, cp, []model.UsageEvent{event}, false); e != nil {
		t.Fatal(e)
	}
	if e = db.Commit(ctx, cp, []model.UsageEvent{event}, false); e != nil {
		t.Fatal(e)
	}
	sum, e := db.Summary(ctx, usage.Filter{})
	if e != nil || sum.Records != 1 || sum.Tokens.Total() != 120 || sum.UnknownPrices != 1 {
		t.Fatalf("%+v %v", sum, e)
	}
	p, ok, e := db.Checkpoint(ctx, "k")
	if e != nil || !ok || p.Offset != 100 {
		t.Fatal(p, e)
	}
	if e = db.Commit(ctx, usage.Checkpoint{Key: "k", Path: "/session"}, nil, true); e != nil {
		t.Fatal(e)
	}
	sum, e = db.Summary(ctx, usage.Filter{})
	if e != nil || sum.Records != 0 {
		t.Fatal(sum, e)
	}
}
func TestFutureSchemaRejected(t *testing.T) {
	ctx := context.Background()
	p := filepath.Join(t.TempDir(), "usage.sqlite3")
	db, e := Open(ctx, p)
	if e != nil {
		t.Fatal(e)
	}
	_, e = db.db.Execute(ctx, "PRAGMA user_version = 999")
	if e != nil {
		t.Fatal(e)
	}
	db.Close()
	if d, e := Open(ctx, p); e == nil {
		d.Close()
		t.Fatal("accepted future schema")
	}
}

// This integration must run against the real MyGo SQLite plugin. It deliberately
// keeps the legacy WAL writer open, so a main-file-only copy would lose data.
func TestLegacyWALHistoryDedupResetAndRollback(t *testing.T) {
	ctx := context.Background()
	root := t.TempDir()
	source := filepath.Join(root, "legacy.sqlite3")
	old, e := sqlite.Open(ctx, source, sqlite.OpenOptions{})
	if e != nil {
		t.Fatal(e)
	}
	defer old.Close()
	if _, e = old.Query(ctx, "PRAGMA journal_mode=WAL"); e != nil {
		t.Fatal(e)
	}
	if _, e = old.Query(ctx, "PRAGMA wal_autocheckpoint=0"); e != nil {
		t.Fatal(e)
	}
	for _, sql := range []string{
		`CREATE TABLE usage_events(record_id TEXT PRIMARY KEY,event_timestamp TEXT,input_tokens INTEGER,cached_input_tokens INTEGER,output_tokens INTEGER,reasoning_output_tokens INTEGER,attributed_account_id TEXT,session_id TEXT,source_file TEXT,cwd TEXT,model TEXT,legacy_extra TEXT)`,
		`CREATE TABLE account_usage_snapshot(workspace_id TEXT PRIMARY KEY,payload TEXT)`,
	} {
		if _, e = old.Execute(ctx, sql); e != nil {
			t.Fatal(e)
		}
	}
	legacyID := "/old/session.jsonl:3:120"
	if _, e = old.Execute(ctx, `INSERT INTO usage_events VALUES(?,?,?,?,?,?,?,?,?,?,?,?)`, legacyID, "2026-10-01T10:00:00Z", 100, 40, 20, 5, "profile-a", "session-a", "/old/session.jsonl", "/workspace", "model-a", "retain-future-column"); e != nil {
		t.Fatal(e)
	}
	if _, e = old.Execute(ctx, `INSERT INTO account_usage_snapshot VALUES(?,?)`, "workspace-a", `{"important":"retained"}`); e != nil {
		t.Fatal(e)
	}
	d, e := Open(ctx, filepath.Join(root, "new", "usage.sqlite3"))
	if e != nil {
		t.Fatal(e)
	}
	defer d.Close()
	if e = d.Stage(ctx, source, "import-a"); e != nil {
		t.Fatal(e)
	}
	sum, e := d.Summary(ctx, usage.Filter{})
	if e != nil || sum.Records != 0 {
		t.Fatal("staging exposed", sum, e)
	}
	if e = d.Publish(ctx, "import-a"); e != nil {
		t.Fatal(e)
	}
	if e = d.Publish(ctx, "import-a"); e != nil {
		t.Fatal("publish is not idempotent", e)
	}
	sum, e = d.Summary(ctx, usage.Filter{})
	if e != nil || sum.Records != 1 || sum.Tokens.Total() != 120 {
		t.Fatal(sum, e)
	}
	raw, e := d.LegacyRows(ctx, "import-a", "usage_events", 0, 100)
	if e != nil || len(raw) != 1 || !strings.Contains(string(raw[0]), "retain-future-column") {
		t.Fatal(raw, e)
	}
	accounts, e := d.LegacyRows(ctx, "import-a", "account_usage_snapshot", 0, 100)
	if e != nil || len(accounts) != 1 {
		t.Fatal(accounts, e)
	}
	event := model.UsageEvent{ID: "reindexed-event", LegacyRecordID: legacyID, ProfileID: "profile-a", SessionID: "session-a", Workspace: "/workspace", Model: "model-a", Timestamp: time.Date(2026, 10, 1, 10, 0, 0, 0, time.UTC), Tokens: model.Tokens{Input: 100, CachedInput: 40, Output: 20, ReasoningOutput: 5}}
	cp := usage.Checkpoint{Key: "reindex-source", Path: "/old/session.jsonl", Offset: 200}
	if e = d.Commit(ctx, cp, []model.UsageEvent{event}, false); e != nil {
		t.Fatal(e)
	}
	sum, e = d.Summary(ctx, usage.Filter{})
	if e != nil || sum.Records != 1 {
		t.Fatal("reindex double counted", sum, e)
	}
	if e = d.Commit(ctx, cp, nil, true); e != nil {
		t.Fatal(e)
	}
	sum, e = d.Summary(ctx, usage.Filter{})
	if e != nil || sum.Records != 1 {
		t.Fatal("reset deleted DB-only history", sum, e)
	}
	if e = d.RollbackImport(ctx, "import-a"); e != nil {
		t.Fatal(e)
	}
	sum, e = d.Summary(ctx, usage.Filter{})
	if e != nil || sum.Records != 0 {
		t.Fatal("rollback", sum, e)
	}
	rows, e := old.Query(ctx, "SELECT count(*) FROM usage_events")
	if e != nil || integer(rows.Values[0][0]) != 1 {
		t.Fatal("source mutated", e)
	}
}

func TestInsightsDoesNotUseVisiblePageOnly(t *testing.T) {
	ctx := context.Background()
	d, e := Open(ctx, filepath.Join(t.TempDir(), "usage.sqlite3"))
	if e != nil {
		t.Fatal(e)
	}
	defer d.Close()
	cp := usage.Checkpoint{Key: "insights", Path: "/test/insights.jsonl"}
	events := []model.UsageEvent{}
	for i, effort := range []string{"medium", "low", "medium"} {
		events = append(events, model.UsageEvent{ID: fmt.Sprint(i), ProfileID: "p", SessionID: "session", Timestamp: time.Date(2026, 10, 27, 0, 0, i, 0, time.UTC), Effort: effort, AgentRole: "reviewer", Tokens: model.Tokens{Input: 10}})
	}
	if e = d.Commit(ctx, cp, events, false); e != nil {
		t.Fatal(e)
	}
	v, e := d.Insights(ctx, usage.Filter{ProfileID: "p", Limit: 1, Offset: 2})
	if e != nil || v.TotalSkillsUsed != 3 || v.MostUsedReasoningPercent == nil || *v.MostUsedReasoningPercent != 2.0/3.0 {
		t.Fatal(v, e)
	}
}

// Acceptance for CMD-035/036/037/039/042/043: the same Usage scope must
// produce consistent overview, calls, threads and activity. It requires the
// REAL official MyGo SQLite plugin, so is never counted in offline passes.
func TestScopedDashboardQueriesAndPaginationAgainstMyGoSQLite(t *testing.T) {
	ctx := context.Background()
	db, err := Open(ctx, filepath.Join(t.TempDir(), "usage.sqlite3"))
	if err != nil {
		t.Fatal(err)
	}
	defer db.Close()
	at := time.Date(2026, 10, 20, 12, 0, 0, 0, time.UTC)
	events := []model.UsageEvent{
		{ID: "a", ProfileID: "profile-a", SessionID: "session-1", Model: "m", Timestamp: at, Tokens: model.Tokens{Input: 100, CachedInput: 40, Output: 20}},
		{ID: "b", ProfileID: "profile-a", SessionID: "session-1", Model: "m", Timestamp: at.Add(time.Second), Tokens: model.Tokens{Input: 50, CachedInput: 10, Output: 10}},
		{ID: "c", ProfileID: "profile-b", SessionID: "session-2", Model: "m", Timestamp: at.Add(2 * time.Second), Tokens: model.Tokens{Input: 800, Output: 20}},
	}
	if err = db.Commit(ctx, usage.Checkpoint{Key: "source-a", Path: "/tmp/source-a.jsonl"}, events, false); err != nil {
		t.Fatal(err)
	}
	scope := usage.Filter{ProfileID: "profile-a", After: at.Add(-time.Second), Before: at.Add(24 * time.Hour), Limit: 1}
	sum, err := db.Summary(ctx, scope)
	if err != nil || sum.Records != 2 || sum.Threads != 1 || sum.Tokens.Total() != 180 || sum.Tokens.CachedInput != 50 {
		t.Fatalf("summary leakage: %+v %v", sum, err)
	}
	first, err := db.Calls(ctx, scope)
	if err != nil || first.Total != 2 || len(first.Items) != 1 || first.Items[0].ProfileID != "profile-a" {
		t.Fatalf("calls page: %+v %v", first, err)
	}
	scope.Offset = 1
	second, err := db.Calls(ctx, scope)
	if err != nil || second.Total != 2 || len(second.Items) != 1 || second.Items[0].ID == first.Items[0].ID {
		t.Fatalf("calls next page: %+v %v", second, err)
	}
	threads, err := db.ThreadPage(ctx, scope)
	if err != nil || threads.Total != 1 || len(threads.Items) != 0 {
		t.Fatalf("thread page: %+v %v", threads, err)
	}
	scope.Offset = 0
	threads, err = db.ThreadPage(ctx, scope)
	if err != nil || threads.Total != 1 || len(threads.Items) != 1 || threads.Items[0].ProfileID != "profile-a" {
		t.Fatalf("thread scope: %+v %v", threads, err)
	}
	daily, err := db.Daily(ctx, scope, time.UTC)
	if err != nil || len(daily) < 1 || daily[0].Records != 2 || daily[0].Tokens != 180 {
		t.Fatalf("activity scope: %+v %v", daily, err)
	}
}

func TestNativeUsageCompactAndResetPreserveRebuildableReadModel(t *testing.T) {
	ctx := context.Background()
	db, err := Open(ctx, filepath.Join(t.TempDir(), "usage.sqlite3"))
	if err != nil {
		t.Fatal(err)
	}
	defer db.Close()
	event := model.UsageEvent{ID: "one", ProfileID: "p", SessionID: "s", Timestamp: time.Date(2026, 10, 20, 0, 0, 0, 0, time.UTC), Tokens: model.Tokens{Input: 10, Output: 2}}
	if err = db.Commit(ctx, usage.Checkpoint{Key: "source", Path: "/tmp/test.jsonl"}, []model.UsageEvent{event}, false); err != nil {
		t.Fatal(err)
	}
	if err = db.Compact(ctx); err != nil {
		t.Fatal(err)
	}
	before, err := db.Summary(ctx, usage.Filter{})
	if err != nil || before.Records != 1 {
		t.Fatalf("compact lost usage: %+v %v", before, err)
	}
	if err = db.Reset(ctx); err != nil {
		t.Fatal(err)
	}
	after, err := db.Summary(ctx, usage.Filter{})
	if err != nil || after.Records != 0 {
		t.Fatalf("reset failed: %+v %v", after, err)
	}
}
