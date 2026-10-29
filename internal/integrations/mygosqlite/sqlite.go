// Package mygosqlite is a LAM read-model repository on the official MyGo plugin.
// It is not a database/sql replacement and does not contain a SQLite engine.
package mygosqlite

import (
	"context"
	"encoding/json"
	"fmt"
	"github.com/egoist/mygo/plugins/sqlite"
	"lam.local/lam2/internal/fault"
	"lam.local/lam2/internal/model"
	"lam.local/lam2/internal/storage"
	"lam.local/lam2/internal/usage"
	"os"
	"path/filepath"
	"strconv"
	"time"
)

type DB struct {
	db   *sqlite.DB
	path string
}

const schemaVersion = 2

func Open(ctx context.Context, path string) (*DB, error) {
	if err := storage.EnsurePrivateDir(filepath.Dir(path)); err != nil {
		return nil, err
	}
	if err := storage.RejectSymlinks(path); err != nil {
		return nil, err
	}
	conn, err := sqlite.Open(ctx, path, sqlite.OpenOptions{BusyTimeout: 5 * time.Second})
	if err != nil {
		return nil, fault.Wrap("DATABASE_UNAVAILABLE", "Cannot open usage database", err)
	}
	d := &DB{conn, path}
	rows, err := conn.Query(ctx, "PRAGMA user_version")
	if err != nil {
		conn.Close()
		return nil, err
	}
	if len(rows.Values) != 1 || integer(rows.Values[0][0]) > schemaVersion {
		conn.Close()
		return nil, fault.New("SCHEMA_UNSUPPORTED", "Database was created by a newer LAM version")
	}
	// The database is an isolated read model. It is never opened over legacy data.
	for _, sql := range []string{"PRAGMA journal_mode=WAL", "PRAGMA foreign_keys=ON"} {
		if _, err = conn.Query(ctx, sql); err != nil {
			conn.Close()
			return nil, err
		}
	}
	statements := []sqlite.Statement{
		{SQL: `CREATE TABLE IF NOT EXISTS usage_events(id TEXT PRIMARY KEY,profile_id TEXT NOT NULL,session_id TEXT NOT NULL,workspace TEXT NOT NULL,model TEXT NOT NULL,timestamp TEXT NOT NULL,input_tokens INTEGER NOT NULL,cached_input_tokens INTEGER NOT NULL,output_tokens INTEGER NOT NULL,reasoning_output_tokens INTEGER NOT NULL,cost REAL,price_version TEXT NOT NULL,payload TEXT NOT NULL)`},
		{SQL: `CREATE TABLE IF NOT EXISTS source_files(source_key TEXT PRIMARY KEY,path TEXT NOT NULL UNIQUE,checkpoint TEXT NOT NULL)`},
		{SQL: `CREATE TABLE IF NOT EXISTS source_occurrences(source_key TEXT NOT NULL,event_id TEXT NOT NULL,PRIMARY KEY(source_key,event_id))`},
		{SQL: `CREATE INDEX IF NOT EXISTS usage_profile_time ON usage_events(profile_id,timestamp,id)`},
		{SQL: `CREATE INDEX IF NOT EXISTS usage_time ON usage_events(timestamp,id)`},
		{SQL: `CREATE INDEX IF NOT EXISTS usage_workspace ON usage_events(workspace,model,timestamp)`},
		{SQL: `CREATE INDEX IF NOT EXISTS occurrence_event ON source_occurrences(event_id)`},
		{SQL: `CREATE TABLE IF NOT EXISTS account_usage_snapshot(profile_id TEXT PRIMARY KEY,fetched_at TEXT NOT NULL,payload TEXT NOT NULL)`},
		{SQL: `CREATE TABLE IF NOT EXISTS diagnostic_snapshots(id TEXT PRIMARY KEY,created_at TEXT NOT NULL,code TEXT NOT NULL,payload TEXT NOT NULL)`},
		{SQL: `CREATE TABLE IF NOT EXISTS event_aliases(alias TEXT PRIMARY KEY,event_id TEXT NOT NULL)`},
		{SQL: `CREATE TABLE IF NOT EXISTS legacy_imports(batch_id TEXT PRIMARY KEY,source_path TEXT NOT NULL,status TEXT NOT NULL,created_at TEXT NOT NULL)`},
		{SQL: `CREATE TABLE IF NOT EXISTS legacy_rows(batch_id TEXT NOT NULL,table_name TEXT NOT NULL,row_key TEXT NOT NULL,payload TEXT NOT NULL,PRIMARY KEY(batch_id,table_name,row_key))`},
		{SQL: `CREATE TABLE IF NOT EXISTS legacy_event_stage(batch_id TEXT NOT NULL,id TEXT NOT NULL,profile_id TEXT NOT NULL,session_id TEXT NOT NULL,workspace TEXT NOT NULL,model TEXT NOT NULL,timestamp TEXT NOT NULL,input_tokens INTEGER NOT NULL,cached_input_tokens INTEGER NOT NULL,output_tokens INTEGER NOT NULL,reasoning_output_tokens INTEGER NOT NULL,cost REAL,price_version TEXT NOT NULL,payload TEXT NOT NULL,legacy_record_id TEXT NOT NULL,PRIMARY KEY(batch_id,id))`},
		{SQL: `PRAGMA user_version = 2`},
	}
	if _, err = conn.Transaction(ctx, statements); err != nil {
		conn.Close()
		return nil, fault.Wrap("DATABASE_UNAVAILABLE", "Cannot initialize usage database", err)
	}
	if err = os.Chmod(path, 0600); err != nil {
		conn.Close()
		return nil, err
	}
	return d, nil
}
func (d *DB) Close() error { return d.db.Close() }
func (d *DB) Checkpoint(ctx context.Context, key string) (usage.Checkpoint, bool, error) {
	var cp usage.Checkpoint
	r, e := d.db.Query(ctx, "SELECT checkpoint FROM source_files WHERE source_key = ?", key)
	if e != nil {
		return cp, false, e
	}
	if len(r.Values) == 0 {
		return cp, false, nil
	}
	if e = json.Unmarshal([]byte(text(r.Values[0][0])), &cp); e != nil {
		return cp, false, fault.Wrap("USAGE_INDEX_STALE", "Invalid checkpoint; rebuild the index", e)
	}
	return cp, true, nil
}
func eventStatement(e model.UsageEvent) (sqlite.Statement, error) {
	if err := e.Tokens.Validate(); err != nil {
		return sqlite.Statement{}, err
	}
	b, err := json.Marshal(e)
	if err != nil {
		return sqlite.Statement{}, err
	}
	var cost any
	if e.Cost != nil {
		cost = *e.Cost
	}
	return sqlite.Statement{SQL: `INSERT INTO usage_events(id,profile_id,session_id,workspace,model,timestamp,input_tokens,cached_input_tokens,output_tokens,reasoning_output_tokens,cost,price_version,payload) VALUES(COALESCE((SELECT event_id FROM event_aliases WHERE alias=?),?),?,?,?,?,?,?,?,?,?,?,?,?) ON CONFLICT(id) DO NOTHING`, Args: []any{legacyAlias(e), e.ID, e.ProfileID, e.SessionID, e.Workspace, e.Model, e.Timestamp.UTC().Format(usage.SQLTimeLayout), e.Tokens.Input, e.Tokens.CachedInput, e.Tokens.Output, e.Tokens.ReasoningOutput, cost, e.PriceVersion, string(b)}}, nil
}
func (d *DB) Commit(ctx context.Context, c usage.Checkpoint, events []model.UsageEvent, reset bool) error {
	b, err := json.Marshal(c)
	if err != nil {
		return err
	}
	stmts := make([]sqlite.Statement, 0, len(events)*2+4)
	if reset {
		stmts = append(stmts, sqlite.Statement{SQL: "DELETE FROM source_occurrences WHERE source_key = ?", Args: []any{c.Key}})
	}
	for _, e := range events {
		stmt, err := eventStatement(e)
		if err != nil {
			return err
		}
		stmts = append(stmts, stmt, sqlite.Statement{SQL: "INSERT OR IGNORE INTO source_occurrences(source_key,event_id) VALUES(?,COALESCE((SELECT event_id FROM event_aliases WHERE alias=?),?))", Args: []any{c.Key, legacyAlias(e), e.ID}})
		if e.LegacyRecordID != "" {
			stmts = append(stmts, sqlite.Statement{SQL: "INSERT OR IGNORE INTO event_aliases(alias,event_id) VALUES(?,?)", Args: []any{legacyAlias(e), e.ID}})
		}
	}
	stmts = append(stmts, sqlite.Statement{SQL: `INSERT INTO source_files(source_key,path,checkpoint) VALUES(?,?,?) ON CONFLICT(source_key) DO UPDATE SET path=excluded.path,checkpoint=excluded.checkpoint`, Args: []any{c.Key, c.Path, string(b)}})
	if reset {
		stmts = append(stmts, sqlite.Statement{SQL: `DELETE FROM usage_events WHERE NOT EXISTS(SELECT 1 FROM source_occurrences WHERE event_id=usage_events.id)`})
	}
	_, err = d.db.Transaction(ctx, stmts)
	return err
}
func (d *DB) Summary(ctx context.Context, f usage.Filter) (usage.Summary, error) {
	w, args := f.SQL()
	rows, e := d.db.Query(ctx, `SELECT count(*),count(DISTINCT COALESCE(NULLIF(json_extract(payload,'$.threadKey'),''),profile_id||char(0)||session_id)),count(cost),COALESCE(sum(input_tokens),0),COALESCE(sum(cached_input_tokens),0),COALESCE(sum(output_tokens),0),COALESCE(sum(reasoning_output_tokens),0),COALESCE(sum(cost),0) FROM usage_events WHERE `+w, args...)
	var s usage.Summary
	if e != nil {
		return s, e
	}
	if len(rows.Values) != 1 {
		return s, fault.New("DATABASE_INVALID", "Invalid aggregate response")
	}
	v := rows.Values[0]
	s.Records = integer(v[0])
	s.Threads = integer(v[1])
	s.KnownPrices = integer(v[2])
	s.UnknownPrices = s.Records - s.KnownPrices
	s.Tokens = model.Tokens{Input: integer(v[3]), CachedInput: integer(v[4]), Output: integer(v[5]), ReasoningOutput: integer(v[6])}
	s.EstimatedUSD = number(v[7])
	return s, nil
}
func (d *DB) Calls(ctx context.Context, f usage.Filter) (model.Page[model.UsageEvent], error) {
	page := model.Page[model.UsageEvent]{Items: []model.UsageEvent{}, Offset: max(0, f.Offset), Limit: f.Limit}
	if page.Limit <= 0 || page.Limit > 1000 {
		page.Limit = 100
	}
	w, args := f.SQL()
	count, e := d.db.Query(ctx, "SELECT count(*) FROM usage_events WHERE "+w, args...)
	if e != nil {
		return page, e
	}
	page.Total = int(integer(count.Values[0][0]))
	args = append(args, page.Limit, page.Offset)
	r, e := d.db.Query(ctx, "SELECT id,payload FROM usage_events WHERE "+w+" ORDER BY "+f.CallOrderSQL()+" LIMIT ? OFFSET ?", args...)
	if e != nil {
		return page, e
	}
	for _, row := range r.Values {
		var event model.UsageEvent
		if e = json.Unmarshal([]byte(text(row[1])), &event); e != nil {
			return page, e
		}
		event.ID = text(row[0])
		page.Items = append(page.Items, event)
	}
	return page, nil
}
func (d *DB) Daily(ctx context.Context, f usage.Filter, loc *time.Location) ([]usage.Daily, error) {
	end := f.Before
	if end.IsZero() {
		end = time.Now().Add(time.Nanosecond)
	}
	start := f.After
	if start.IsZero() {
		start = usage.DefaultActivityStart(end)
	}
	windows := usage.DayWindows(start, end, loc)
	if len(windows) > 366 {
		return nil, fault.New("DATE_RANGE_TOO_LARGE", "Select up to one year for daily charts")
	}
	// Native MyGo SQLite buffers query results. For UTC activity, aggregate
	// the entire range in one bounded GROUP BY instead of 365 sequential
	// Summary calls (one per calendar day), which stalls large Usage views.
	if loc == nil || loc == time.UTC || loc.String() == "UTC" {
		q := f
		q.After, q.Before = start, end
		where, args := q.SQL()
		rows, e := d.db.Query(ctx, `SELECT substr(timestamp,1,10),count(*),COALESCE(sum(input_tokens+output_tokens),0),COALESCE(sum(cost),0) FROM usage_events WHERE `+where+` GROUP BY substr(timestamp,1,10)`, args...)
		if e != nil {
			return nil, e
		}
		byDate := make(map[string]usage.Daily, len(rows.Values))
		for _, v := range rows.Values {
			date := text(v[0])
			byDate[date] = usage.Daily{Date: date, Records: integer(v[1]), Tokens: integer(v[2]), EstimatedUSD: number(v[3])}
		}
		out := make([]usage.Daily, 0, len(windows))
		for _, w := range windows {
			d, ok := byDate[w.Date]
			if !ok {
				d = usage.Daily{Date: w.Date}
			}
			out = append(out, d)
		}
		return out, nil
	}
	// Other timezone callers retain exact DST-boundary semantics; the
	// production activity panel deliberately uses UTC like the legacy app.
	out := make([]usage.Daily, 0, len(windows))
	for _, w := range windows {
		q := f
		q.After = w.Start
		if q.After.Before(start) {
			q.After = start
		}
		q.Before = w.End
		if q.Before.After(end) {
			q.Before = end
		}
		s, e := d.Summary(ctx, q)
		if e != nil {
			return nil, e
		}
		out = append(out, usage.Daily{Date: w.Date, Tokens: s.Tokens.Total(), Records: s.Records, EstimatedUSD: s.EstimatedUSD})
	}
	return out, nil
}

type Thread struct {
	Key, Name                                      string
	ProfileID, SessionID, Workspace, Model, LastAt string
	Records, Tokens                                int64
	EstimatedUSD                                   float64
}

func (d *DB) ThreadPage(ctx context.Context, f usage.Filter) (model.Page[Thread], error) {
	page := model.Page[Thread]{Items: []Thread{}, Limit: f.Limit, Offset: max(0, f.Offset)}
	if page.Limit < 1 || page.Limit > 1000 {
		page.Limit = 100
	}
	w, args := f.SQL()
	group := "COALESCE(NULLIF(json_extract(payload,'$.threadKey'),''),profile_id||char(0)||session_id)"
	count, e := d.db.Query(ctx, "SELECT count(DISTINCT "+group+") FROM usage_events WHERE "+w, args...)
	if e != nil {
		return page, e
	}
	page.Total = int(integer(count.Values[0][0]))
	args = append(args, page.Limit, page.Offset)
	r, e := d.db.Query(ctx, "SELECT "+group+",COALESCE(max(json_extract(payload,'$.threadName')),''),profile_id,session_id,max(workspace),max(model),max(timestamp),count(*),sum(input_tokens+output_tokens),COALESCE(sum(cost),0) FROM usage_events WHERE "+w+" GROUP BY "+group+" ORDER BY max(timestamp) DESC,"+group+" LIMIT ? OFFSET ?", args...)
	if e != nil {
		return page, e
	}
	for _, v := range r.Values {
		page.Items = append(page.Items, Thread{text(v[0]), text(v[1]), text(v[2]), text(v[3]), text(v[4]), text(v[5]), text(v[6]), integer(v[7]), integer(v[8]), number(v[9])})
	}
	return page, nil
}
func (d *DB) Threads(ctx context.Context, f usage.Filter) ([]Thread, error) {
	p, e := d.ThreadPage(ctx, f)
	return p.Items, e
}
func (d *DB) Reprice(ctx context.Context, cards []model.RateCard) error {
	last := ""
	for {
		r, e := d.db.Query(ctx, "SELECT id,payload FROM usage_events WHERE id > ? ORDER BY id LIMIT 500", last)
		if e != nil {
			return e
		}
		if len(r.Values) == 0 {
			return nil
		}
		stmts := make([]sqlite.Statement, 0, len(r.Values))
		for _, v := range r.Values {
			var ev model.UsageEvent
			if e = json.Unmarshal([]byte(text(v[1])), &ev); e != nil {
				return e
			}
			usage.Price(&ev, cards)
			b, e := json.Marshal(ev)
			if e != nil {
				return e
			}
			var cost any
			if ev.Cost != nil {
				cost = *ev.Cost
			}
			last = text(v[0])
			stmts = append(stmts, sqlite.Statement{SQL: "UPDATE usage_events SET cost=?,price_version=?,payload=? WHERE id=?", Args: []any{cost, ev.PriceVersion, string(b), last}})
		}
		if _, e = d.db.Transaction(ctx, stmts); e != nil {
			return e
		}
	}
}
func (d *DB) Reset(ctx context.Context) error {
	// SQLite-only legacy records are not reconstructible from JSONL. Keep their
	// source occurrences and aliases even when the rebuildable index is cleared.
	_, e := d.db.Transaction(ctx, []sqlite.Statement{{SQL: "DELETE FROM source_occurrences WHERE source_key NOT LIKE 'legacy:%'"}, {SQL: "DELETE FROM usage_events WHERE NOT EXISTS(SELECT 1 FROM source_occurrences WHERE event_id=usage_events.id)"}, {SQL: "DELETE FROM event_aliases WHERE NOT EXISTS(SELECT 1 FROM usage_events WHERE id=event_aliases.event_id)"}, {SQL: "DELETE FROM source_files"}})
	return e
}
func (d *DB) Compact(ctx context.Context) error { _, e := d.db.Execute(ctx, "VACUUM"); return e }
func (d *DB) SaveAccountUsage(ctx context.Context, profile string, raw json.RawMessage) error {
	if !json.Valid(raw) {
		return fault.New("USAGE_INVALID", "Account usage response is invalid")
	}
	_, e := d.db.Execute(ctx, `INSERT INTO account_usage_snapshot VALUES(?,?,?) ON CONFLICT(profile_id) DO UPDATE SET fetched_at=excluded.fetched_at,payload=excluded.payload`, profile, time.Now().UTC().Format(time.RFC3339Nano), string(raw))
	return e
}
func (d *DB) AccountUsageSnapshot(ctx context.Context, profile string) (json.RawMessage, time.Time, bool, error) {
	r, e := d.db.Query(ctx, "SELECT fetched_at,payload FROM account_usage_snapshot WHERE profile_id=?", profile)
	if e != nil {
		return nil, time.Time{}, false, e
	}
	if len(r.Values) == 0 {
		return nil, time.Time{}, false, nil
	}
	at, e := time.Parse(time.RFC3339Nano, text(r.Values[0][0]))
	if e != nil {
		return nil, time.Time{}, false, fault.New("USAGE_INVALID", "Cached account timestamp is invalid")
	}
	return json.RawMessage(text(r.Values[0][1])), at, true, nil
}
func text(v any) string {
	switch x := v.(type) {
	case string:
		return x
	case []byte:
		return string(x)
	case nil:
		return ""
	default:
		return fmt.Sprint(v)
	}
}
func integer(v any) int64 {
	switch x := v.(type) {
	case int64:
		return x
	case int:
		return int64(x)
	case float64:
		return int64(x)
	default:
		n, _ := strconv.ParseInt(text(v), 10, 64)
		return n
	}
}
func number(v any) float64 {
	switch x := v.(type) {
	case float64:
		return x
	case int64:
		return float64(x)
	default:
		n, _ := strconv.ParseFloat(text(v), 64)
		return n
	}
}

// Insights consumes the entire filtered result, not just the visible Calls page.
// Batches are bounded; LAM's data read lock excludes its own concurrent reindex.
func (d *DB) Insights(ctx context.Context, f usage.Filter) (usage.Insights, error) {
	a := usage.NewInsightsAccumulator()
	f.Offset = 0
	for {
		if f.Limit = f.DetailBatch(f.Offset); f.Limit == 0 {
			break
		}
		if e := ctx.Err(); e != nil {
			return usage.Insights{}, e
		}
		p, e := d.Calls(ctx, f)
		if e != nil {
			return usage.Insights{}, e
		}
		for _, v := range p.Items {
			a.Add(v)
		}
		f.Offset += len(p.Items)
		if len(p.Items) == 0 || f.Offset >= p.Total {
			break
		}
	}
	return a.Result(), nil
}

func (d *DB) Diagnostics(ctx context.Context, f usage.Filter) (usage.Diagnostics, error) {
	a := usage.NewDiagnosticsAccumulator()
	f.Offset = 0
	for {
		if f.Limit = f.DetailBatch(f.Offset); f.Limit == 0 {
			break
		}
		if e := ctx.Err(); e != nil {
			return usage.Diagnostics{}, e
		}
		p, e := d.Calls(ctx, f)
		if e != nil {
			return usage.Diagnostics{}, e
		}
		for _, v := range p.Items {
			a.Add(v)
		}
		f.Offset += len(p.Items)
		if len(p.Items) == 0 || f.Offset >= p.Total {
			break
		}
	}
	result := a.Result()
	r, e := d.db.Query(ctx, `SELECT checkpoint FROM source_files`)
	if e != nil {
		return result, e
	}
	for _, row := range r.Values {
		if len(row) == 0 {
			continue
		}
		var cp usage.Checkpoint
		if json.Unmarshal([]byte(text(row[0])), &cp) != nil {
			return result, fault.New("USAGE_INDEX_STALE", "Cannot parse source diagnostics checkpoint")
		}
		if len(cp.ParserDiagnostics) == 0 && cp.Diagnostics > 0 {
			result.ParserDiagnostics["legacy_unclassified"] += int64(cp.Diagnostics)
			continue
		}
		for category, count := range cp.ParserDiagnostics {
			result.ParserDiagnostics[category] += count
		}
		result.SkippedEvents += cp.SkippedEvents
	}
	// Parser checkpoints cover all indexed files. Unlike the event aggregates,
	// these diagnostics are index-wide, matching legacy refresh diagnostics.
	r, e = d.db.Query(ctx, `SELECT payload FROM diagnostic_snapshots WHERE id='last-refresh'`)
	if e != nil {
		return result, e
	}
	if len(r.Values) > 0 {
		var v struct {
			Error string `json:"error"`
		}
		if json.Unmarshal([]byte(text(r.Values[0][0])), &v) == nil {
			result.LastRefreshError = v.Error
		}
	}
	return result, nil
}
func (d *DB) RecordRefresh(ctx context.Context, code string) error {
	raw, _ := json.Marshal(map[string]string{"error": code})
	_, e := d.db.Execute(ctx, `INSERT INTO diagnostic_snapshots(id,created_at,code,payload) VALUES('last-refresh',?,?,?) ON CONFLICT(id) DO UPDATE SET created_at=excluded.created_at,code=excluded.code,payload=excluded.payload`, time.Now().UTC().Format(time.RFC3339Nano), code, string(raw))
	return e
}
