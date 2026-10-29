package migration

import (
	"context"
	"encoding/json"
	"errors"
	"testing"
)

func TestLegacyUsagePreservesHistoricalFields(t *testing.T) {
	row := map[string]any{"record_id": "/a/rollout.jsonl:17:120", "session_id": "s", "event_timestamp": "2026-07-09T10:11:12.123Z", "source_file": "/a/rollout.jsonl", "line_number": int64(17), "cumulative_total_tokens": int64(120), "attributed_account_id": "account-1", "workspace_id": "work-1", "cwd": "/projects/x", "model": "m", "input_tokens": int64(100), "cached_input_tokens": int64(40), "output_tokens": int64(20), "reasoning_output_tokens": int64(5), "estimated_cost_usd": 0.125, "pricing_confidence": "exact", "thread_key": "thread-parent", "parent_session_id": "parent", "is_archived": int64(1), "unknown_future_field": "keep"}
	event, e := DecodeUsageRow(row)
	if e != nil {
		t.Fatal(e)
	}
	if event.ProfileID != "account-1" || event.Tokens.Uncached() != 60 || event.Tokens.Total() != 120 || event.Cost == nil || *event.Cost != 0.125 || !event.Archived || event.ThreadKey != "thread-parent" || event.LineNumber != 17 || event.LegacyRecordID != row["record_id"] {
		t.Fatalf("bad mapping: %+v", event)
	}
	var raw map[string]any
	if json.Unmarshal(event.Legacy, &raw) != nil || raw["unknown_future_field"] != "keep" {
		t.Fatal("discarded legacy fields")
	}
	delete(row, "attributed_account_id")
	delete(row, "estimated_cost_usd")
	event, e = DecodeUsageRow(row)
	if e != nil || event.ProfileID != "unattributed" || event.Cost != nil {
		t.Fatal("invented attribution or price", e, event)
	}
	row["input_tokens"] = int64(-1)
	if _, e = DecodeUsageRow(row); e == nil {
		t.Fatal("accepted invalid counters")
	}
}

type fakeUsageReader struct {
	version int64
	changed bool
	tables  []string
	rows    map[string][]map[string]any
}

func (f *fakeUsageReader) Version(context.Context) (int64, error) {
	f.version++
	if f.changed && f.version > 1 {
		return 2, nil
	}
	return 1, nil
}
func (f *fakeUsageReader) Tables(context.Context) ([]string, error) { return f.tables, nil }
func (f *fakeUsageReader) Rows(_ context.Context, table string, offset, limit int) ([]map[string]any, error) {
	rows := f.rows[table]
	if offset >= len(rows) {
		return nil, nil
	}
	return rows[offset:min(offset+limit, len(rows))], nil
}

type fakeUsageSink struct {
	begun, aborted bool
	count, events  int
}

func (f *fakeUsageSink) Begin(context.Context, string, string) error { f.begun = true; return nil }
func (f *fakeUsageSink) Append(_ context.Context, _ string, rows []LegacyRow) error {
	f.count += len(rows)
	for _, r := range rows {
		if r.Event != nil {
			f.events++
		}
	}
	return nil
}
func (f *fakeUsageSink) Seal(context.Context, string) error  { return nil }
func (f *fakeUsageSink) Abort(context.Context, string) error { f.aborted = true; return nil }
func TestLogicalImportIncludesAllTablesAndRejectsMovingSnapshot(t *testing.T) {
	row := map[string]any{"record_id": "r", "session_id": "s", "event_timestamp": "2026-07-09T00:00:00Z", "input_tokens": int64(1), "output_tokens": int64(2)}
	f := &fakeUsageReader{tables: []string{"usage_events", "account_usage_snapshot", "account_usage_daily_buckets", "future_table"}, rows: map[string][]map[string]any{"usage_events": {row}, "account_usage_snapshot": {{"snapshot_id": "a"}}, "account_usage_daily_buckets": {{"tokens": int64(99)}}, "future_table": {{"opaque": "preserved"}}}}
	sink := &fakeUsageSink{}
	report, e := StageLegacyUsage(context.Background(), "batch", "db", f, sink)
	if e != nil || report.Events != 1 || report.Rows != 4 || sink.aborted {
		t.Fatal(report, e, sink)
	}
	f.changed = true
	f.version = 0
	sink = &fakeUsageSink{}
	_, e = StageLegacyUsage(context.Background(), "changed", "db", f, sink)
	if e == nil || !sink.aborted {
		t.Fatal("inconsistent snapshot was not aborted", e)
	}
	f.changed = false
	f.version = 0
	row["event_timestamp"] = "invalid"
	_, e = StageLegacyUsage(context.Background(), "bad", "db", f, &fakeUsageSink{})
	if e == nil || errors.Is(e, context.Canceled) {
		t.Fatal("invalid row silently skipped", e)
	}
}
