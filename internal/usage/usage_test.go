package usage

import (
	"context"
	"encoding/json"
	"lam.local/lam2/internal/model"
	"lam.local/lam2/internal/storage"
	"os"
	"path/filepath"
	"testing"
	"time"
)

func TestCountersDedupResetAndAliases(t *testing.T) {
	var c CounterState
	raw := []byte(`{"total_token_usage":{"input_tokens":100,"cached_input_tokens":60,"output_tokens":20,"reasoning_output_tokens":5},"last_token_usage":{"input_tokens":100,"cached_input_tokens":60,"output_tokens":20,"reasoning_output_tokens":5}}`)
	d, ok, e := Delta(raw, &c)
	if e != nil || !ok || d.Uncached() != 40 || d.Total() != 120 {
		t.Fatal(d, ok, e)
	}
	_, ok, e = Delta(raw, &c)
	if e != nil || ok {
		t.Fatal("duplicate counted", e)
	}
	d, ok, e = Delta([]byte(`{"total_token_usage":{"prompt_tokens":150,"cached_prompt_tokens":80,"completion_tokens":40,"reasoning_tokens":10}}`), &c)
	if e != nil || !ok || d.Input != 50 || d.Output != 20 {
		t.Fatal(d, e)
	}
	d, ok, e = Delta([]byte(`{"total_token_usage":{"input_tokens":5,"output_tokens":2}}`), &c)
	if e != nil || !ok || d.Total() != 7 || c.Resets != 1 {
		t.Fatal(d, c, e)
	}
}
func TestNegativeOrInconsistentCountersRejected(t *testing.T) {
	for _, raw := range []string{`{"total_token_usage":{"input_tokens":-1}}`, `{"total_token_usage":{"input_tokens":1,"cached_input_tokens":2}}`, `{"total_token_usage":{"input_tokens":1.2}}`} {
		var c CounterState
		if _, _, e := Delta([]byte(raw), &c); e == nil {
			t.Fatal(raw)
		}
	}
}
func TestPriceVersionAndUnknown(t *testing.T) {
	cards := []model.RateCard{{Model: "m", Version: "a", Currency: "USD", EffectiveAt: time.Date(2025, 1, 1, 0, 0, 0, 0, time.UTC), InputPerMillion: 2, CachedPerMillion: 1, OutputPerMillion: 4}, {Model: "m", Version: "b", Currency: "USD", EffectiveAt: time.Date(2026, 1, 1, 0, 0, 0, 0, time.UTC), InputPerMillion: 3, CachedPerMillion: 1, OutputPerMillion: 6}}
	e := model.UsageEvent{Model: "m", Timestamp: time.Date(2025, 6, 1, 0, 0, 0, 0, time.UTC), Tokens: model.Tokens{Input: 1000000, CachedInput: 500000, Output: 100000}}
	Price(&e, cards)
	if e.Cost == nil || *e.Cost != 1.9 || e.PriceVersion != "a" {
		t.Fatal(e)
	}
	e.Model = "unknown"
	Price(&e, cards)
	if e.Cost != nil || e.PricingConfidence != "unknown" {
		t.Fatal(e)
	}
}

type memoryRepo struct {
	cp     map[string]Checkpoint
	events map[string]model.UsageEvent
	fail   bool
}

func (m *memoryRepo) Checkpoint(ctx context.Context, key string) (Checkpoint, bool, error) {
	c, ok := m.cp[key]
	return c, ok, nil
}
func (m *memoryRepo) Commit(ctx context.Context, cp Checkpoint, events []model.UsageEvent, reset bool) error {
	if m.fail {
		return os.ErrPermission
	}
	if m.cp == nil {
		m.cp = map[string]Checkpoint{}
		m.events = map[string]model.UsageEvent{}
	}
	if reset {
		for k, v := range m.events {
			if v.SourceFile == cp.Path {
				delete(m.events, k)
			}
		}
	}
	for _, v := range events {
		m.events[v.ID] = v
	}
	m.cp[cp.Key] = cp
	return nil
}

type fixtureParser struct{}

func (fixtureParser) ParseLine(b []byte) ([]Observation, error) {
	var envelope struct {
		Type      string
		Timestamp time.Time
		Payload   struct {
			Model, CWD string
			Type       string
			Info       json.RawMessage
		}
	}
	if e := json.Unmarshal(b, &envelope); e != nil {
		return nil, e
	}
	return []Observation{{Kind: envelope.Type, Timestamp: envelope.Timestamp, Model: envelope.Payload.Model, Workspace: envelope.Payload.CWD, Usage: envelope.Payload.Info}}, nil
}
func TestIncrementalPartialLineAndAtomicCheckpoint(t *testing.T) {
	home := t.TempDir()
	path := filepath.Join(home, "sessions", "a.jsonl")
	os.MkdirAll(filepath.Dir(path), 0700)
	line := `{"type":"event_msg","timestamp":"2026-10-20T12:00:00Z","payload":{"type":"token_count","info":{"total_token_usage":{"input_tokens":10,"output_tokens":3}}}}` + "\n"
	os.WriteFile(path, []byte(line+`{"type":`), 0600)
	repo := &memoryRepo{}
	ix := Indexer{Repo: repo, Parser: func() Parser { return fixtureParser{} }, LockRoot: t.TempDir()}
	p := model.Profile{ID: "p", Home: home}
	ses := model.Session{ID: "s", Path: path, Model: "m"}
	result, e := ix.Index(context.Background(), p, ses, nil, nil)
	if e != nil || result.Events != 1 || result.Offset != int64(len(line)) {
		t.Fatal(result, e)
	}
	first := repo.cp[result.Key]
	result, e = ix.Index(context.Background(), p, ses, nil, nil)
	if e != nil || result.Events != 0 || len(repo.events) != 1 {
		t.Fatal(result, e)
	}
	os.WriteFile(path, []byte(line+line), 0600)
	result, e = ix.Index(context.Background(), p, ses, nil, nil)
	if e != nil || len(repo.events) != 1 {
		t.Fatal(result, e)
	}
	repo.fail = true
	os.WriteFile(path, []byte(line+line+line), 0600)
	_, e = ix.Index(context.Background(), p, ses, nil, nil)
	if e == nil {
		t.Fatal("commit failure ignored")
	}
	if repo.cp[result.Key].Offset == int64(len(line)*3) {
		t.Fatal("checkpoint advanced on failure")
	}
	if first.Offset != int64(len(line)) {
		t.Fatal(first)
	}
}
func TestFileTruncationRebuildsWithoutDoubling(t *testing.T) {
	home := t.TempDir()
	path := filepath.Join(home, "sessions", "a.jsonl")
	os.MkdirAll(filepath.Dir(path), 0700)
	line := func(n string) []byte {
		return []byte(`{"type":"event_msg","timestamp":"2026-10-20T12:00:00Z","payload":{"info":{"total_token_usage":{"input_tokens":` + n + `}}}}` + "\n")
	}
	os.WriteFile(path, line("100"), 0600)
	repo := &memoryRepo{}
	ix := Indexer{Repo: repo, Parser: func() Parser { return fixtureParser{} }, LockRoot: t.TempDir()}
	p := model.Profile{ID: "p", Home: home}
	s := model.Session{ID: "s", Path: path}
	ix.Index(context.Background(), p, s, nil, nil)
	os.WriteFile(path, line("5"), 0600)
	r, e := ix.Index(context.Background(), p, s, nil, nil)
	if e != nil || !r.Rebuilt || len(repo.events) != 1 {
		t.Fatal(r, e)
	}
	for _, v := range repo.events {
		if v.Tokens.Input != 5 {
			t.Fatal(v)
		}
	}
}
func TestUsageCannotEscapeHome(t *testing.T) {
	ix := Indexer{Repo: &memoryRepo{}, Parser: func() Parser { return fixtureParser{} }, LockRoot: t.TempDir()}
	_, e := ix.Index(context.Background(), model.Profile{ID: "p", Home: t.TempDir()}, model.Session{Path: filepath.Join(t.TempDir(), "auth.json")}, nil, nil)
	if e == nil {
		t.Fatal("outside file accepted")
	}
}
func TestDSTCalendarDays(t *testing.T) {
	loc, e := time.LoadLocation("America/Los_Angeles")
	if e != nil {
		t.Fatal(e)
	}
	days := DayWindows(time.Date(2026, 3, 7, 0, 0, 0, 0, loc), time.Date(2026, 3, 10, 0, 0, 0, 0, loc), loc)
	if len(days) != 3 || days[1].End.Sub(days[1].Start) != 23*time.Hour {
		t.Fatal(days)
	}
}
func TestMalformedUTF8Diagnostic(t *testing.T) {
	home := t.TempDir()
	path := filepath.Join(home, "sessions", "a.jsonl")
	os.MkdirAll(filepath.Dir(path), 0700)
	os.WriteFile(path, []byte{0xff, '\n'}, 0600)
	ix := Indexer{Repo: &memoryRepo{}, Parser: func() Parser { return fixtureParser{} }, LockRoot: t.TempDir()}
	r, e := ix.Index(context.Background(), model.Profile{ID: "p", Home: home}, model.Session{ID: "s", Path: path}, nil, nil)
	if e != nil || r.Diagnostics != 1 {
		t.Fatal(r, e)
	}
}
func TestLedgerFingerprintChanges(t *testing.T) {
	if storage.Hash([]byte("a")) == storage.Hash([]byte("b")) {
		t.Fatal("hash collision")
	}
}

func TestFilterTimestampPrecisionAndOrdering(t *testing.T) {
	base := time.Date(2026, 10, 27, 10, 0, 0, 0, time.UTC)
	_, args := (Filter{After: base, Before: base.Add(time.Millisecond)}).SQL()
	if len(args) != 2 || args[0] != "2026-10-27T10:00:00.000000000Z" || args[1] != "2026-10-27T10:00:00.001000000Z" {
		t.Fatalf("SQL timestamps must use a fixed-width UTC representation: %#v", args)
	}
	if args[0].(string) >= args[1].(string) {
		t.Fatal("timestamp ordering differs from chronological order")
	}
}

func TestLegacyLastUsageIsAuthoritativeAndBuiltInPrices(t *testing.T) {
	var c CounterState
	delta, ok, e := Delta([]byte(`{"total_token_usage":{"input_tokens":10000,"output_tokens":2000},"last_token_usage":{"input_tokens":100,"cached_input_tokens":40,"output_tokens":20}}`), &c)
	if e != nil || !ok || delta.Total() != 120 {
		t.Fatal("first recorded call is not the whole session total", delta, e)
	}
	ev := model.UsageEvent{Model: "gpt-5.4", Timestamp: time.Now(), Tokens: model.Tokens{Input: 128000, Output: 100}}
	Price(&ev, nil)
	if ev.Cost == nil || *ev.Cost != 0.3215 || ev.PriceVersion != "legacy-0.4.4" {
		t.Fatal(ev)
	}
	ev.Tokens.Input = 128001
	Price(&ev, nil)
	if ev.Cost == nil || *ev.Cost != 0.642255 {
		t.Fatal("long-context boundary", ev)
	}
	ev.Model = "codex-auto-review"
	Price(&ev, nil)
	if ev.Cost == nil || ev.PricingConfidence != "legacy-alias-estimate" {
		t.Fatal(ev)
	}
}

func TestActivityAndRawHistoryBoundaries(t *testing.T) {
	days := []Daily{{Date: "2026-10-24", Records: 1, Tokens: 10}, {Date: "2026-10-25", Records: 1, Tokens: 20}, {Date: "2026-10-26", Records: 0}, {Date: "2026-10-27", Records: 2, Tokens: 7}}
	a := AnalyzeActivity(days, time.Date(2026, 10, 27, 12, 0, 0, 0, time.UTC))
	if a.CurrentStreak != 1 || a.LongestStreak != 2 || a.ActiveDays != 3 || a.Peak.Tokens != 20 {
		t.Fatal(a)
	}
	if _, e := ReadRawCall(context.Background(), model.UsageEvent{SourceFile: "/etc/passwd"}, []string{t.TempDir()}); e == nil {
		t.Fatal("arbitrary file disclosure")
	}
}

func TestPATTimelineAttributionDoesNotAssignCurrentIdentityToHistory(t *testing.T) {
	t0 := time.Date(2026, 1, 1, 0, 0, 0, 0, time.UTC)
	t1 := t0.Add(time.Hour)
	periods := []model.PATPeriod{{ProfileID: "a", StartedAt: t0, EndedAt: &t1}, {ProfileID: "b", StartedAt: t1}}
	if id := PATOwner(periods, t0.Add(time.Minute), ""); id != "a" {
		t.Fatal(id)
	}
	if id := PATOwner(periods, t1, ""); id != "b" {
		t.Fatal(id)
	}
	if id := PATOwner(periods, t0.Add(-time.Minute), ""); id != "unattributed" {
		t.Fatal(id)
	}
	periods = append(periods, model.PATPeriod{ProfileID: "conflict", StartedAt: t0})
	if id := PATOwner(periods, t1, ""); id != "unattributed" {
		t.Fatal("ambiguous timeline must not guess", id)
	}
}

func TestPATIsolatedUsageNotAssignedToSharedActiveIdentity(t *testing.T) {
	home := t.TempDir()
	path := filepath.Join(home, "sessions", "pat.jsonl")
	os.MkdirAll(filepath.Dir(path), 0700)
	line := `{"type":"event_msg","timestamp":"2026-10-20T12:00:00Z","payload":{"type":"token_count","info":{"last_token_usage":{"input_tokens":10,"output_tokens":3}}}}` + "\n"
	os.WriteFile(path, []byte(line), 0600)
	repo := &memoryRepo{}
	ix := Indexer{Repo: repo, Parser: func() Parser { return fixtureParser{} }, LockRoot: t.TempDir(), PATTimeline: []model.PATPeriod{{ProfileID: "other", StartedAt: time.Date(2026, 1, 1, 0, 0, 0, 0, time.UTC)}}}
	_, e := ix.Index(context.Background(), model.Profile{ID: "isolated", Home: home, AuthMode: "pat"}, model.Session{ID: "s", Path: path}, nil, nil)
	if e != nil {
		t.Fatal(e)
	}
	for _, v := range repo.events {
		if v.ProfileID != "isolated" {
			t.Fatal(v.ProfileID)
		}
	}
	if len(repo.events) != 1 {
		t.Fatal("no usage event")
	}
}
