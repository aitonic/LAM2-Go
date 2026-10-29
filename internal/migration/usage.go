package migration

import (
	"context"
	"encoding/json"
	"fmt"
	"math"
	"strconv"
	"strings"
	"time"

	"lam.local/lam2/internal/fault"
	"lam.local/lam2/internal/model"
	"lam.local/lam2/internal/storage"
)

// LegacyRow retains every column, including columns added by legacy releases.
// The normalized read model is additional, never a replacement for the source row.
type LegacyRow struct {
	Table, Key string
	Payload    json.RawMessage
	Event      *model.UsageEvent
}
type UsageImportReport struct{ Rows, Events, Tables int }
type LegacyUsageReader interface {
	Version(context.Context) (int64, error)
	Tables(context.Context) ([]string, error)
	Rows(context.Context, string, int, int) ([]map[string]any, error)
}
type LegacyUsageSink interface {
	Begin(context.Context, string, string) error
	Append(context.Context, string, []LegacyRow) error
	Seal(context.Context, string) error
	Abort(context.Context, string) error
}

// StageLegacyUsage reads bounded pages into an invisible staging area. The same
// SQLite connection's data_version must not change across the whole read. This
// is an idle-source logical snapshot, not a claim of live Backup API support.
func StageLegacyUsage(ctx context.Context, id, path string, source LegacyUsageReader, sink LegacyUsageSink) (report UsageImportReport, err error) {
	version, err := source.Version(ctx)
	if err != nil {
		return report, err
	}
	tables, err := source.Tables(ctx)
	if err != nil {
		return report, err
	}
	hasUsage := false
	for _, table := range tables {
		if table == "usage_events" {
			hasUsage = true
		}
	}
	if !hasUsage {
		return report, fault.New("LEGACY_SCHEMA_UNSUPPORTED", "Legacy database has no usage_events table")
	}
	if err = sink.Begin(ctx, id, path); err != nil {
		return report, err
	}
	defer func() {
		if err != nil {
			cleanup, cancel := context.WithTimeout(context.Background(), 5*time.Second)
			defer cancel()
			_ = sink.Abort(cleanup, id)
		}
	}()
	for _, table := range tables {
		if strings.HasPrefix(table, "sqlite_") {
			continue
		}
		report.Tables++
		for offset := 0; ; offset += 500 {
			if err = ctx.Err(); err != nil {
				return report, err
			}
			var rows []map[string]any
			rows, err = source.Rows(ctx, table, offset, 500)
			if err != nil {
				return report, err
			}
			if len(rows) == 0 {
				break
			}
			batch := make([]LegacyRow, 0, len(rows))
			for n, row := range rows {
				var raw []byte
				raw, err = json.Marshal(row)
				if err != nil {
					return report, err
				}
				r := LegacyRow{Table: table, Key: fmt.Sprintf("%012d", offset+n), Payload: raw}
				if table == "usage_events" {
					var event model.UsageEvent
					event, err = DecodeUsageRow(row)
					if err != nil {
						return report, err
					}
					r.Event = &event
					report.Events++
				}
				batch = append(batch, r)
				report.Rows++
			}
			if err = sink.Append(ctx, id, batch); err != nil {
				return report, err
			}
		}
	}
	var current int64
	current, err = source.Version(ctx)
	if err != nil {
		return report, err
	}
	if current != version {
		return report, fault.New("LEGACY_DATABASE_CHANGED", "Legacy writer changed the database during import; stop it and retry")
	}
	err = sink.Seal(ctx, id)
	return report, err
}

func DecodeUsageRow(row map[string]any) (model.UsageEvent, error) {
	get := func(k string) string {
		switch v := row[k].(type) {
		case nil:
			return ""
		case string:
			return v
		case []byte:
			return string(v)
		default:
			return fmt.Sprint(v)
		}
	}
	integer := func(k string) (int64, error) {
		v := row[k]
		if v == nil {
			return 0, nil
		}
		switch n := v.(type) {
		case int64:
			return n, nil
		case int:
			return int64(n), nil
		case float64:
			if math.Trunc(n) != n || math.Abs(n) > 9007199254740991 {
				return 0, fmt.Errorf("inexact integer column %s", k)
			}
			return int64(n), nil
		case bool:
			if n {
				return 1, nil
			}
			return 0, nil
		default:
			return strconv.ParseInt(get(k), 10, 64)
		}
	}
	var e model.UsageEvent
	id := get("record_id")
	if id == "" {
		return e, fault.New("LEGACY_ROW_INVALID", "Legacy usage record has no stable record_id")
	}
	e.ID = "legacy-" + storage.Hash([]byte(id))
	e.LegacyRecordID = id
	e.SessionID = get("session_id")
	e.SourceFile = get("source_file")
	e.ProfileID = get("attributed_account_id")
	if e.ProfileID == "" {
		e.ProfileID = "unattributed"
	}
	e.Workspace = get("cwd")
	e.WorkspaceID = get("workspace_id")
	e.Model = get("model")
	e.Effort = get("effort")
	e.SubagentType = get("subagent_type")
	e.AgentRole = get("agent_role")
	e.AgentNickname = get("agent_nickname")
	e.ModelContextWindow, _ = integer("model_context_window")
	e.ThreadKey = get("thread_key")
	e.ThreadName = get("thread_name")
	e.AttributionSource = get("attribution_source")
	stamp := get("event_timestamp")
	var err error
	e.Timestamp, err = time.Parse(time.RFC3339Nano, stamp)
	if err != nil {
		return e, fault.New("LEGACY_ROW_INVALID", "Invalid legacy event timestamp; source row is not discarded")
	}
	for key, dst := range map[string]*int64{"input_tokens": &e.Tokens.Input, "cached_input_tokens": &e.Tokens.CachedInput, "output_tokens": &e.Tokens.Output, "reasoning_output_tokens": &e.Tokens.ReasoningOutput, "line_number": &e.LineNumber, "cumulative_total_tokens": &e.CumulativeTotal} {
		*dst, err = integer(key)
		if err != nil {
			return e, fault.New("LEGACY_ROW_INVALID", "Invalid legacy usage counter")
		}
	}
	if err = e.Tokens.Validate(); err != nil {
		return e, fault.New("LEGACY_ROW_INVALID", "Inconsistent legacy token accounting; source row is not discarded")
	}
	archived, err := integer("is_archived")
	if err != nil {
		return e, err
	}
	e.Archived = archived != 0
	if row["estimated_cost_usd"] != nil {
		cost, err := strconv.ParseFloat(get("estimated_cost_usd"), 64)
		if err != nil || math.IsNaN(cost) || math.IsInf(cost, 0) || cost < 0 {
			return e, fault.New("LEGACY_ROW_INVALID", "Invalid legacy price")
		}
		e.Cost = &cost
		e.PriceVersion = "legacy-import"
	}
	e.PricingConfidence = get("pricing_confidence")
	if e.PricingConfidence == "" {
		e.PricingConfidence = "unknown"
	}
	e.Legacy, err = json.Marshal(row)
	return e, err
}
