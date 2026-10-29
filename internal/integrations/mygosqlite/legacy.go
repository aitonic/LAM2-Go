package mygosqlite

import (
	"context"
	"encoding/json"
	"os"
	"path/filepath"
	"sort"
	"strings"
	"time"

	"github.com/egoist/mygo/plugins/sqlite"
	"lam.local/lam2/internal/fault"
	"lam.local/lam2/internal/migration"
	"lam.local/lam2/internal/model"
	"lam.local/lam2/internal/storage"
	"lam.local/lam2/internal/usage"
)

func legacyAlias(e model.UsageEvent) string {
	if e.LegacyRecordID == "" {
		return "record:" + e.ID
	}
	return "legacy-record:" + e.LegacyRecordID
}

type legacyReader struct {
	db     *sqlite.DB
	tables map[string]bool
}

func (r *legacyReader) Version(ctx context.Context) (int64, error) {
	v, e := r.db.Query(ctx, "PRAGMA data_version")
	if e != nil {
		return 0, e
	}
	if len(v.Values) != 1 {
		return 0, fault.New("DATABASE_INVALID", "Cannot read source snapshot version")
	}
	return integer(v.Values[0][0]), nil
}
func (r *legacyReader) Tables(ctx context.Context) ([]string, error) {
	v, e := r.db.Query(ctx, "SELECT name FROM sqlite_schema WHERE type='table' AND name NOT LIKE 'sqlite_%' ORDER BY name")
	if e != nil {
		return nil, e
	}
	r.tables = map[string]bool{}
	out := []string{}
	for _, row := range v.Values {
		n := text(row[0])
		r.tables[n] = true
		out = append(out, n)
	}
	return out, nil
}
func (r *legacyReader) Rows(ctx context.Context, table string, offset, limit int) ([]map[string]any, error) {
	if !r.tables[table] {
		return nil, fault.New("LEGACY_SCHEMA_UNSUPPORTED", "Table is not part of the captured schema")
	}
	quoted := `"` + strings.ReplaceAll(table, `"`, `""`) + `"`
	cols, e := r.db.Query(ctx, "PRAGMA table_info("+quoted+")")
	if e != nil {
		return nil, e
	}
	type key struct {
		pos  int64
		name string
	}
	keys := []key{}
	for _, col := range cols.Values {
		if integer(col[5]) > 0 {
			keys = append(keys, key{integer(col[5]), text(col[1])})
		}
	}
	sort.Slice(keys, func(i, j int) bool { return keys[i].pos < keys[j].pos })
	order := "rowid"
	if len(keys) > 0 {
		parts := []string{}
		for _, k := range keys {
			parts = append(parts, `"`+strings.ReplaceAll(k.name, `"`, `""`)+`"`)
		}
		order = strings.Join(parts, ",")
	}
	rows, e := r.db.Query(ctx, "SELECT * FROM "+quoted+" ORDER BY "+order+" LIMIT ? OFFSET ?", limit, offset)
	if e != nil {
		return nil, e
	}
	out := make([]map[string]any, 0, len(rows.Values))
	for _, values := range rows.Values {
		row := map[string]any{}
		for i, col := range rows.Columns {
			row[col] = values[i]
		}
		out = append(out, row)
	}
	return out, nil
}

// Stage uses MyGo's real read-only connection, which reads committed WAL rows.
// Source data must stay unchanged during pagination; hashes and data_version
// are both checked. No raw main-file-only copy, ATTACH or hand-written driver.
func (d *DB) Stage(ctx context.Context, path, id string) error {
	if !storage.ValidID(id) {
		return fault.New("PLAN_INVALID", "Invalid import ID")
	}
	if err := storage.RejectSymlinks(path); err != nil {
		return err
	}
	if filepath.Clean(path) == filepath.Clean(d.path) {
		return fault.New("MIGRATION_ROOT_INVALID", "Cannot import the destination database")
	}
	before, err := databaseFingerprint(path)
	if err != nil {
		return err
	}
	src, err := sqlite.Open(ctx, path, sqlite.OpenOptions{ReadOnly: true, BusyTimeout: 5 * time.Second})
	if err != nil {
		return err
	}
	defer src.Close()
	_, err = migration.StageLegacyUsage(ctx, id, path, &legacyReader{db: src}, d)
	if err != nil {
		return err
	}
	after, err := databaseFingerprint(path)
	if err != nil || before != after {
		cleanup, cancel := context.WithTimeout(context.Background(), 5*time.Second)
		defer cancel()
		_ = d.Abort(cleanup, id)
		return fault.New("LEGACY_DATABASE_CHANGED", "Legacy database or WAL changed; stop its writer and retry")
	}
	return nil
}
func databaseFingerprint(path string) (string, error) {
	parts := []string{}
	for _, p := range []string{path, path + "-wal"} {
		hash, e := storage.FileHash(p)
		if os.IsNotExist(e) && p != path {
			hash = "absent"
		} else if e != nil {
			return "", e
		}
		parts = append(parts, hash)
	}
	return storage.Hash([]byte(strings.Join(parts, ":"))), nil
}
func (d *DB) Begin(ctx context.Context, id, path string) error {
	rows, e := d.db.Query(ctx, "SELECT status FROM legacy_imports WHERE batch_id=?", id)
	if e != nil {
		return e
	}
	if len(rows.Values) > 0 && text(rows.Values[0][0]) == "committed" {
		return fault.New("MIGRATION_ALREADY_APPLIED", "Legacy import is already published")
	}
	_, e = d.db.Transaction(ctx, []sqlite.Statement{{SQL: "DELETE FROM legacy_rows WHERE batch_id=?", Args: []any{id}}, {SQL: "DELETE FROM legacy_event_stage WHERE batch_id=?", Args: []any{id}}, {SQL: "INSERT INTO legacy_imports(batch_id,source_path,status,created_at) VALUES(?,?,?,?) ON CONFLICT(batch_id) DO UPDATE SET status=excluded.status,source_path=excluded.source_path", Args: []any{id, path, "staging", time.Now().UTC().Format(time.RFC3339Nano)}}})
	return e
}
func (d *DB) Append(ctx context.Context, id string, rows []migration.LegacyRow) error {
	statements := []sqlite.Statement{}
	for _, r := range rows {
		statements = append(statements, sqlite.Statement{SQL: "INSERT INTO legacy_rows VALUES(?,?,?,?)", Args: []any{id, r.Table, r.Key, string(r.Payload)}})
		if r.Event == nil {
			continue
		}
		e := *r.Event
		b, err := json.Marshal(e)
		if err != nil {
			return err
		}
		var cost any
		if e.Cost != nil {
			cost = *e.Cost
		}
		statements = append(statements, sqlite.Statement{SQL: "INSERT INTO legacy_event_stage VALUES(?,?,?,?,?,?,?,?,?,?,?,?,?,?,?)", Args: []any{id, e.ID, e.ProfileID, e.SessionID, e.Workspace, e.Model, e.Timestamp.UTC().Format(usage.SQLTimeLayout), e.Tokens.Input, e.Tokens.CachedInput, e.Tokens.Output, e.Tokens.ReasoningOutput, cost, e.PriceVersion, string(b), e.LegacyRecordID}})
	}
	_, e := d.db.Transaction(ctx, statements)
	return e
}
func (d *DB) Seal(ctx context.Context, id string) error {
	_, e := d.db.Execute(ctx, "UPDATE legacy_imports SET status='prepared' WHERE batch_id=? AND status='staging'", id)
	return e
}
func (d *DB) Abort(ctx context.Context, id string) error {
	_, e := d.db.Transaction(ctx, []sqlite.Statement{{SQL: "DELETE FROM legacy_rows WHERE batch_id=? AND EXISTS(SELECT 1 FROM legacy_imports WHERE batch_id=? AND status!='committed')", Args: []any{id, id}}, {SQL: "DELETE FROM legacy_event_stage WHERE batch_id=?", Args: []any{id}}, {SQL: "DELETE FROM legacy_imports WHERE batch_id=? AND status!='committed'", Args: []any{id}}})
	return e
}

const publishLegacyEventsSQL = `INSERT OR IGNORE INTO usage_events(id,profile_id,session_id,workspace,model,timestamp,input_tokens,cached_input_tokens,output_tokens,reasoning_output_tokens,cost,price_version,payload) SELECT COALESCE((SELECT event_id FROM event_aliases WHERE alias='legacy-record:'||s.legacy_record_id),s.id),profile_id,session_id,workspace,model,timestamp,input_tokens,cached_input_tokens,output_tokens,reasoning_output_tokens,cost,price_version,payload FROM legacy_event_stage s WHERE batch_id=?`
const publishLegacyOccurrencesSQL = `INSERT OR IGNORE INTO source_occurrences(source_key,event_id) SELECT ?,COALESCE((SELECT event_id FROM event_aliases WHERE alias='legacy-record:'||s.legacy_record_id),s.id) FROM legacy_event_stage s WHERE batch_id=?`
const publishLegacyAliasesSQL = `INSERT OR IGNORE INTO event_aliases(alias,event_id) SELECT 'legacy-record:'||legacy_record_id,id FROM legacy_event_stage WHERE batch_id=?`

func (d *DB) Publish(ctx context.Context, id string) error {
	rows, e := d.db.Query(ctx, "SELECT status FROM legacy_imports WHERE batch_id=?", id)
	if e != nil {
		return e
	}
	if len(rows.Values) != 1 {
		return fault.New("MIGRATION_STATE_INVALID", "Prepared usage import is missing")
	}
	status := text(rows.Values[0][0])
	if status == "committed" {
		return nil
	}
	if status != "prepared" {
		return fault.New("MIGRATION_STATE_INVALID", "Usage import is not verified")
	}
	_, e = d.db.Transaction(ctx, []sqlite.Statement{
		{SQL: publishLegacyEventsSQL, Args: []any{id}},
		{SQL: publishLegacyOccurrencesSQL, Args: []any{"legacy:" + id, id}},
		{SQL: publishLegacyAliasesSQL, Args: []any{id}},
		{SQL: "UPDATE legacy_imports SET status='committed' WHERE batch_id=?", Args: []any{id}},
		{SQL: "DELETE FROM legacy_event_stage WHERE batch_id=?", Args: []any{id}},
	})
	return e
}
func (d *DB) RollbackImport(ctx context.Context, id string) error {
	_, e := d.db.Transaction(ctx, []sqlite.Statement{
		{SQL: "DELETE FROM source_occurrences WHERE source_key=?", Args: []any{"legacy:" + id}},
		{SQL: "DELETE FROM usage_events WHERE NOT EXISTS(SELECT 1 FROM source_occurrences WHERE event_id=usage_events.id)"},
		{SQL: "DELETE FROM event_aliases WHERE NOT EXISTS(SELECT 1 FROM usage_events WHERE id=event_aliases.event_id)"},
		{SQL: "DELETE FROM legacy_event_stage WHERE batch_id=?", Args: []any{id}},
		{SQL: "DELETE FROM legacy_rows WHERE batch_id=?", Args: []any{id}},
		{SQL: "DELETE FROM legacy_imports WHERE batch_id=?", Args: []any{id}},
	})
	return e
}
func (d *DB) LegacyRows(ctx context.Context, batch, table string, offset, limit int) ([]json.RawMessage, error) {
	if limit <= 0 || limit > 500 {
		limit = 100
	}
	r, e := d.db.Query(ctx, `SELECT r.payload FROM legacy_rows r JOIN legacy_imports i ON i.batch_id=r.batch_id AND i.status='committed' WHERE r.batch_id=? AND r.table_name=? ORDER BY row_key LIMIT ? OFFSET ?`, batch, table, limit, max(offset, 0))
	if e != nil {
		return nil, e
	}
	out := []json.RawMessage{}
	for _, row := range r.Values {
		out = append(out, json.RawMessage(text(row[0])))
	}
	return out, nil
}
func (d *DB) LegacySummary(ctx context.Context) ([]map[string]any, error) {
	r, e := d.db.Query(ctx, `SELECT i.batch_id,i.source_path,r.table_name,count(*) FROM legacy_imports i JOIN legacy_rows r ON r.batch_id=i.batch_id WHERE i.status='committed' GROUP BY i.batch_id,r.table_name ORDER BY i.batch_id,r.table_name`)
	if e != nil {
		return nil, e
	}
	out := []map[string]any{}
	for _, v := range r.Values {
		out = append(out, map[string]any{"batchId": text(v[0]), "source": text(v[1]), "table": text(v[2]), "rows": integer(v[3])})
	}
	return out, nil
}

var _ migration.LegacyUsageSink = (*DB)(nil)
var _ migration.LegacyUsageReader = (*legacyReader)(nil)

type LegacyAccountSnapshot struct {
	BatchID     string             `json:"batchId"`
	SnapshotID  string             `json:"snapshotId"`
	FetchedAt   string             `json:"fetchedAt"`
	Attribution string             `json:"attribution"`
	Usage       usage.AccountUsage `json:"usage"`
}

func (d *DB) LegacyAccountSnapshots(ctx context.Context) ([]LegacyAccountSnapshot, error) {
	r, e := d.db.Query(ctx, `SELECT r.batch_id,r.payload FROM legacy_rows r JOIN legacy_imports i ON i.batch_id=r.batch_id AND i.status='committed' WHERE r.table_name='account_usage_snapshot' ORDER BY i.created_at DESC,r.row_key LIMIT 100`)
	if e != nil {
		return nil, e
	}
	out := []LegacyAccountSnapshot{}
	for _, row := range r.Values {
		batch := text(row[0])
		raw := []byte(text(row[1]))
		var meta struct {
			ID string `json:"snapshot_id"`
			At string `json:"fetched_at"`
		}
		if json.Unmarshal(raw, &meta) != nil {
			return nil, fault.New("LEGACY_ROW_INVALID", "Invalid imported account summary")
		}
		var days [][]byte
		for offset := 0; ; offset += 500 {
			page, e := d.db.Query(ctx, `SELECT r.payload FROM legacy_rows r WHERE r.batch_id=? AND r.table_name='account_usage_daily_buckets' AND json_extract(r.payload,'$.snapshot_id')=? ORDER BY json_extract(r.payload,'$.start_date') LIMIT 500 OFFSET ?`, batch, meta.ID, offset)
			if e != nil {
				return nil, e
			}
			for _, v := range page.Values {
				days = append(days, []byte(text(v[0])))
			}
			if len(page.Values) < 500 {
				break
			}
		}
		a, e := usage.ParseLegacyAccountUsage(raw, days)
		if e != nil {
			return nil, e
		}
		out = append(out, LegacyAccountSnapshot{batch, meta.ID, meta.At, "legacy_global_account_unknown", a})
	}
	return out, nil
}
