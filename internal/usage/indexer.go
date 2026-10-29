package usage

import (
	"bufio"
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"lam.local/lam2/internal/fault"
	"lam.local/lam2/internal/handoff"
	"lam.local/lam2/internal/model"
	"lam.local/lam2/internal/storage"
	"os"
	"path/filepath"
	"strings"
	"time"
	"unicode/utf8"
)

// Parser is the narrow agent-parser boundary; no private replacement parser is shipped.
type Parser interface {
	ParseLine([]byte) ([]Observation, error)
}
type Observation struct {
	Kind             string
	Timestamp        time.Time
	Model, Workspace string
	Usage            json.RawMessage
}
type Checkpoint struct {
	RecordCount                                       int64
	LastRecordID                                      string
	MetadataVersion                                   int
	Effort                                            string
	Key, Path, ProfileID, SessionID, Model, Workspace string
	Offset, Size, ModifiedNS                          int64
	HeadLength, TailLength                            int64
	HeadHash, TailHash                                string
	Counter                                           CounterState
	Diagnostics                                       int
	SkippedEvents                                     int64
	ParserDiagnostics                                 map[string]int64
	Lines                                             int64
	Generation                                        int
	UpdatedAt                                         time.Time
}
type Repository interface {
	Checkpoint(context.Context, string) (Checkpoint, bool, error)
	Commit(context.Context, Checkpoint, []model.UsageEvent, bool) error
}
type Indexer struct {
	PATTimeline []model.PATPeriod
	Repo        Repository
	Parser      func() Parser
	LockRoot    string
}
type IndexResult struct {
	Key                 string
	Offset              int64
	Events, Diagnostics int
	Rebuilt             bool
	Partial             bool
}

func (ix *Indexer) Index(ctx context.Context, p model.Profile, s model.Session, cards []model.RateCard, lineage []handoff.Lineage) (IndexResult, error) {
	var result IndexResult
	if ix.Repo == nil || ix.Parser == nil {
		return result, fault.New("USAGE_UNAVAILABLE", "Usage database or parser is unavailable")
	}
	if filepath.Ext(s.Path) != ".jsonl" || !storage.Within(s.Path, []string{filepath.Join(p.Home, "sessions"), filepath.Join(p.Home, "archived_sessions")}) {
		return result, fault.New("PATH_INVALID", "Usage reads only registered session JSONL files")
	}
	if e := storage.RejectSymlinks(s.Path); e != nil {
		return result, e
	}
	key := storage.Hash([]byte(s.Path))
	result.Key = key
	unlock, e := storage.Lock(ctx, filepath.Join(ix.LockRoot, "usage-"+key+".lock"))
	if e != nil {
		return result, e
	}
	defer unlock()
	f, e := os.Open(s.Path)
	if e != nil {
		return result, e
	}
	defer f.Close()
	stat, e := f.Stat()
	if e != nil || !stat.Mode().IsRegular() {
		return result, fault.New("PATH_INVALID", "Usage source is not a regular file")
	}
	cp, exists, e := ix.Repo.Checkpoint(ctx, key)
	if e != nil {
		return result, e
	}
	reset := false
	if exists {
		if cp.MetadataVersion != 2 || cp.Size > stat.Size() || cp.Offset > stat.Size() || (cp.Offset > 0 && cp.Lines == 0) || !matches(f, cp) {
			reset = true
		}
	}
	if !exists || reset {
		gen := cp.Generation + 1
		cp = Checkpoint{MetadataVersion: 2, Key: key, Path: s.Path, ProfileID: p.ID, SessionID: s.ID, Model: s.Model, Workspace: s.CWD, Generation: gen}
		result.Rebuilt = reset
	}
	if exists && !reset && cp.Size == stat.Size() && cp.ModifiedNS == stat.ModTime().UnixNano() {
		result.Offset = cp.Offset
		return result, nil
	}
	if cp.ParserDiagnostics == nil {
		cp.ParserDiagnostics = map[string]int64{}
	}
	// A partial last line can become a complete line on the next append.
	delete(cp.ParserDiagnostics, "partial_trailing_line")
	addDiagnostic := func(code string, skipped bool) {
		if code == "" {
			return
		}
		cp.Diagnostics++
		result.Diagnostics++
		cp.ParserDiagnostics[code]++
		if skipped {
			cp.SkippedEvents++
			cp.ParserDiagnostics["skipped_events"]++
		}
	}
	if _, e = f.Seek(cp.Offset, io.SeekStart); e != nil {
		return result, e
	}
	parser := ix.Parser()
	reader := bufio.NewReaderSize(f, 64<<10)
	events := make([]model.UsageEvent, 0, 500)
	startOffset := cp.Offset
	commit := func() error {
		cp.UpdatedAt = time.Now().UTC()
		cp.Size = stat.Size()
		cp.ModifiedNS = stat.ModTime().UnixNano()
		if err := stamp(f, &cp); err != nil {
			return err
		}
		if err := ix.Repo.Commit(ctx, cp, events, reset); err != nil {
			return err
		}
		result.Events += len(events)
		events = events[:0]
		reset = false
		return nil
	}
	for {
		if ctx.Err() != nil {
			return result, ctx.Err()
		}
		line, n, complete, readErr := readBoundedLine(reader, 8<<20)
		if readErr != nil && readErr != io.EOF {
			return result, readErr
		}
		if !complete {
			result.Partial = n > 0
			if result.Partial {
				cp.ParserDiagnostics["partial_trailing_line"] = 1
			}
			break
		}
		offset := cp.Offset
		cp.Offset += int64(n)
		cp.Lines++
		if line == nil {
			addDiagnostic("oversized_line", true)
			continue
		}
		line = bytes.TrimSpace(line)
		if len(line) == 0 {
			continue
		}
		if !utf8.Valid(line) {
			addDiagnostic("invalid_utf8", false)
			continue
		}
		category, skipped := parserDiagnosticCategory(line)
		if category != "" {
			addDiagnostic(category, skipped)
		}
		if category == "invalid_json" || skipped {
			continue
		}
		obs, err := parser.ParseLine(line)
		if err != nil {
			if category == "" {
				addDiagnostic("parser_error", false)
			}
			continue
		}
		metadata := CallMetadata(line, &cp.Effort)
		for _, o := range obs {
			if o.Model != "" {
				cp.Model = o.Model
			}
			if o.Workspace != "" {
				cp.Workspace = o.Workspace
			}
			if len(o.Usage) == 0 || string(o.Usage) == "null" {
				continue
			}
			delta, ok, err := Delta(o.Usage, &cp.Counter)
			if err != nil {
				addDiagnostic("invalid_token_counters", true)
				continue
			}
			if !ok {
				continue
			}
			if o.Timestamp.IsZero() {
				addDiagnostic("invalid_timestamp", true)
				continue
			}
			owner, sessionID := attribution(p, s, offset, lineage)
			if p.AuthMode == "pat_shared" {
				owner = PATOwner(ix.PATTimeline, o.Timestamp, "")
			}
			v := model.UsageEvent{ID: eventID(owner, sessionID, o.Timestamp, o.Usage), ProfileID: owner, SourceFile: s.Path, SessionID: sessionID, Workspace: cp.Workspace, Model: cp.Model, Timestamp: o.Timestamp.UTC(), Tokens: delta}
			v.Effort, v.SubagentType, v.AgentRole, v.AgentNickname, v.ModelContextWindow = metadata.Effort, metadata.SubagentType, metadata.AgentRole, metadata.AgentNickname, metadata.ModelContextWindow
			v.LineNumber = cp.Lines
			v.CumulativeTotal = cp.Counter.Previous.Total()
			v.LegacyRecordID = fmt.Sprintf("%s:%d:%d", s.Path, cp.Lines, v.CumulativeTotal)
			cp.RecordCount++
			v.ThreadCallIndex = cp.RecordCount
			v.PreviousRecordID = cp.LastRecordID
			cp.LastRecordID = v.ID
			v.ThreadKey = owner + ":" + sessionID
			v.ThreadName = s.Title
			v.Archived = storage.Within(s.Path, []string{filepath.Join(p.Home, "archived_sessions")})
			Price(&v, cards)
			events = append(events, v)
		}
		if len(events) >= 500 {
			if err = commit(); err != nil {
				return result, err
			}
		}
		if readErr == io.EOF {
			break
		}
	}
	if cp.Offset != startOffset || reset || !exists || result.Partial {
		if e = commit(); e != nil {
			return result, e
		}
	}
	result.Offset = cp.Offset
	return result, nil
}
func attribution(p model.Profile, s model.Session, offset int64, records []handoff.Lineage) (string, string) {
	owner, sessionID := p.ID, s.ID
	if p.AuthMode == "pat_shared" {
		owner = "unattributed"
	}
	seen := map[string]bool{}
	for depth := 0; depth < 32; depth++ {
		key := owner + ":" + sessionID
		if seen[key] {
			return "unattributed", sessionID
		}
		seen[key] = true
		found := false
		for _, r := range records {
			if r.TargetProfileID == owner && r.TargetSessionID == sessionID && offset < r.CopiedBytes {
				owner, sessionID = r.SourceProfileID, r.SourceSessionID
				found = true
				break
			}
		}
		if !found {
			break
		}
	}
	return owner, sessionID
}
func matches(f *os.File, c Checkpoint) bool {
	head, e := readAtHash(f, 0, c.HeadLength)
	if e != nil || head != c.HeadHash {
		return false
	}
	tail, e := readAtHash(f, c.Offset-c.TailLength, c.TailLength)
	return e == nil && tail == c.TailHash
}
func stamp(f *os.File, c *Checkpoint) error {
	c.HeadLength = min(c.Offset, 4096)
	c.TailLength = min(c.Offset, 65536)
	var e error
	c.HeadHash, e = readAtHash(f, 0, c.HeadLength)
	if e != nil {
		return e
	}
	c.TailHash, e = readAtHash(f, c.Offset-c.TailLength, c.TailLength)
	return e
}
func readAtHash(f *os.File, offset, n int64) (string, error) {
	if offset < 0 || n < 0 {
		return "", errors.New("invalid file checkpoint")
	}
	b := make([]byte, n)
	if n > 0 {
		if _, e := f.ReadAt(b, offset); e != nil {
			return "", e
		}
	}
	return storage.Hash(b), nil
}
func readBoundedLine(r *bufio.Reader, limit int) ([]byte, int, bool, error) {
	var line []byte
	size := 0
	tooLarge := false
	for {
		part, e := r.ReadSlice('\n')
		size += len(part)
		if size > limit {
			tooLarge = true
			line = nil
		} else if !tooLarge {
			line = append(line, part...)
		}
		if e == bufio.ErrBufferFull {
			continue
		}
		complete := len(part) > 0 && part[len(part)-1] == '\n'
		if tooLarge {
			return nil, size, complete, e
		}
		return line, size, complete, e
	}
}

// SQLTimeLayout is fixed-width so SQLite TEXT ordering matches chronological order.
const SQLTimeLayout = "2006-01-02T15:04:05.000000000Z"

// SafeFilter is used by the SQLite repository; column names never come from user input.
type Filter struct {
	ProfileID, Workspace, Model, SessionID string
	ThreadKey, ArchiveScope                string
	Search, Effort, PricingConfidence      string
	SortKey                                string
	SortAscending                          bool
	After, Before                          time.Time
	Limit, Offset, DetailLimit             int
}

func (f Filter) SQL() (string, []any) {
	parts := []string{"1=1"}
	args := []any{}
	for _, v := range []struct{ column, value string }{{"profile_id", f.ProfileID}, {"workspace", f.Workspace}, {"model", f.Model}, {"session_id", f.SessionID}} {
		if v.value != "" {
			parts = append(parts, v.column+" = ?")
			args = append(args, v.value)
		}
	}
	if f.Effort != "" {
		parts = append(parts, "json_extract(payload,'$.effort') = ?")
		args = append(args, f.Effort)
	}
	if f.PricingConfidence != "" {
		parts = append(parts, "json_extract(payload,'$.pricingConfidence') = ?")
		args = append(args, f.PricingConfidence)
	}
	if strings.TrimSpace(f.Search) != "" {
		// instr accepts the literal search text: SQL wildcard characters and
		// punctuation are never interpolated into a LIKE expression.
		parts = append(parts, "instr(lower(COALESCE(json_extract(payload,'$.threadName'),'') || ' ' || session_id || ' ' || workspace || ' ' || model),lower(?)) > 0")
		args = append(args, strings.TrimSpace(f.Search))
	}
	if f.ThreadKey != "" {
		parts = append(parts, "COALESCE(NULLIF(json_extract(payload,'$.threadKey'),''),profile_id||char(0)||session_id)=?")
		args = append(args, f.ThreadKey)
	}
	if f.ArchiveScope == "active" {
		parts = append(parts, "COALESCE(json_extract(payload,'$.isArchived'),0)=0")
	}
	if f.ArchiveScope == "archived" {
		parts = append(parts, "COALESCE(json_extract(payload,'$.isArchived'),0)=1")
	}
	if !f.After.IsZero() {
		parts = append(parts, "timestamp >= ?")
		args = append(args, f.After.UTC().Format(SQLTimeLayout))
	}
	if !f.Before.IsZero() {
		parts = append(parts, "timestamp < ?")
		args = append(args, f.Before.UTC().Format(SQLTimeLayout))
	}
	return strings.Join(parts, " AND "), args
}

// DetailBatch bounds rows materialized for Insights/Diagnostics independently
// of the 50-row Calls and 100-row Threads UI pages. Zero DetailLimit means
// the original "All calls" option, still fetched in bounded 500-row batches.
func (f Filter) DetailBatch(offset int) int {
	if offset < 0 {
		offset = 0
	}
	if f.DetailLimit <= 0 {
		return 500
	}
	if offset >= f.DetailLimit {
		return 0
	}
	return min(500, f.DetailLimit-offset)
}

// CallOrderSQL is a strict whitelist of the original Usage sortable columns.
// Return expressions are constants; no user-provided identifier enters SQL.
func (f Filter) CallOrderSQL() string {
	col := "timestamp"
	switch f.SortKey {
	case "duration":
		col = "COALESCE(CAST(json_extract(payload,'$.threadCallIndex') AS INTEGER),0)"
	case "gap":
		col = "CASE WHEN LENGTH(COALESCE(json_extract(payload,'$.previousRecordId'),''))>0 THEN 1 ELSE 0 END"
	case "attention":
		col = "MIN(36.0,MAX(0,1.0*(input_tokens+output_tokens)/2500.0)) + CASE WHEN input_tokens>0 THEN MIN(35.0,MAX(0,(0.5-1.0*cached_input_tokens/input_tokens)*70.0)) ELSE 0 END"
	case "total", "usage":
		col = "input_tokens + output_tokens"
	case "cached":
		col = "cached_input_tokens"
	case "uncached":
		col = "input_tokens - cached_input_tokens"
	case "output":
		col = "output_tokens"
	case "reasoning":
		col = "reasoning_output_tokens"
	case "cost":
		col = "cost"
	case "cache":
		col = "CASE WHEN input_tokens > 0 THEN 1.0*cached_input_tokens/input_tokens ELSE 0 END"
	case "context":
		col = "CASE WHEN CAST(json_extract(payload,'$.modelContextWindow') AS INTEGER)>0 THEN 1.0*input_tokens/CAST(json_extract(payload,'$.modelContextWindow') AS INTEGER) ELSE 0 END"
	case "model":
		col = "model"
	case "effort":
		col = "json_extract(payload,'$.effort')"
	case "thread":
		col = "COALESCE(json_extract(payload,'$.threadName'),session_id)"
	case "initiator":
		col = "json_extract(payload,'$.callInitiator')"
	}
	direction := "DESC"
	if f.SortAscending {
		direction = "ASC"
	}
	return col + " " + direction + ",id " + direction
}

type Summary struct {
	Records, Threads, KnownPrices, UnknownPrices int64
	Tokens                                       model.Tokens
	EstimatedUSD                                 float64
}
type Daily struct {
	Date         string
	Tokens       int64
	Records      int64
	EstimatedUSD float64
}

// PATOwner uses the credential active at event time, never the current GUI account.
// Conflicting imported periods remain unknown instead of double assigning usage.
func PATOwner(periods []model.PATPeriod, t time.Time, workspaceID string) string {
	owner := ""
	for _, p := range periods {
		if t.Before(p.StartedAt) || (p.EndedAt != nil && !t.Before(*p.EndedAt)) {
			continue
		}
		if workspaceID != "" && p.WorkspaceID != "" && workspaceID != p.WorkspaceID {
			continue
		}
		if owner != "" && owner != p.ProfileID {
			return "unattributed"
		}
		owner = p.ProfileID
	}
	if owner == "" {
		return "unattributed"
	}
	return owner
}
