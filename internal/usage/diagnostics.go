package usage

import (
	"encoding/json"
	"lam.local/lam2/internal/fault"
	"lam.local/lam2/internal/model"
	"sort"
)

type CacheThread struct {
	ThreadKey                      string `json:"threadKey"`
	InputTokens, CachedInputTokens int64
	CacheRatio                     float64
}
type Diagnostics struct {
	ParserDiagnostics map[string]int64   `json:"parserDiagnostics"`
	SkippedEvents     int64              `json:"skippedEvents"`
	UnknownModels     []string           `json:"unknownModels"`
	LowCacheThreads   []CacheThread      `json:"lowCacheThreads"`
	HighContextCalls  []model.UsageEvent `json:"highContextCalls"`
	LastRefreshError  string             `json:"lastRefreshError,omitempty"`
}
type DiagnosticsAccumulator struct {
	unknown map[string]bool
	threads map[string]CacheThread
	high    []model.UsageEvent
}

func NewDiagnosticsAccumulator() *DiagnosticsAccumulator {
	return &DiagnosticsAccumulator{unknown: map[string]bool{}, threads: map[string]CacheThread{}}
}
func (a *DiagnosticsAccumulator) Add(e model.UsageEvent) {
	if e.Cost == nil {
		a.unknown[e.Model] = true
	}
	key := e.ThreadKey
	if key == "" {
		key = e.ProfileID + ":" + e.SessionID
	}
	t := a.threads[key]
	t.ThreadKey = key
	t.InputTokens += e.Tokens.Input
	t.CachedInputTokens += e.Tokens.CachedInput
	a.threads[key] = t
	if e.ModelContextWindow > 0 && float64(e.Tokens.Input)/float64(e.ModelContextWindow) >= 0.8 {
		a.high = append(a.high, e)
		sort.Slice(a.high, func(i, j int) bool { return a.high[i].Timestamp.After(a.high[j].Timestamp) })
		if len(a.high) > 10 {
			a.high = a.high[:10]
		}
	}
}
func (a *DiagnosticsAccumulator) Result() Diagnostics {
	d := Diagnostics{ParserDiagnostics: map[string]int64{}, UnknownModels: []string{}, LowCacheThreads: []CacheThread{}, HighContextCalls: a.high}
	for k := range a.unknown {
		d.UnknownModels = append(d.UnknownModels, k)
	}
	sort.Strings(d.UnknownModels)
	for _, t := range a.threads {
		if t.InputTokens >= 50000 {
			t.CacheRatio = float64(t.CachedInputTokens) / float64(t.InputTokens)
			if t.CacheRatio < 0.2 {
				d.LowCacheThreads = append(d.LowCacheThreads, t)
			}
		}
	}
	sort.Slice(d.LowCacheThreads, func(i, j int) bool {
		if d.LowCacheThreads[i].InputTokens == d.LowCacheThreads[j].InputTokens {
			return d.LowCacheThreads[i].ThreadKey < d.LowCacheThreads[j].ThreadKey
		}
		return d.LowCacheThreads[i].InputTokens > d.LowCacheThreads[j].InputTokens
	})
	if len(d.LowCacheThreads) > 10 {
		d.LowCacheThreads = d.LowCacheThreads[:10]
	}
	return d
}

// Old LAM stored an account-wide global snapshot without the account ID.
// Keep it explicitly unattributed; never attach it to whichever profile is active.
func ParseLegacyAccountUsage(snapshot []byte, days [][]byte) (AccountUsage, error) {
	var old struct {
		Lifetime *int64 `json:"lifetime_tokens"`
		Peak     *int64 `json:"peak_daily_tokens"`
		Turn     *int64 `json:"longest_running_turn_sec"`
		Current  *int64 `json:"current_streak_days"`
		Longest  *int64 `json:"longest_streak_days"`
	}
	if json.Unmarshal(snapshot, &old) != nil {
		return AccountUsage{}, fault.New("LEGACY_ROW_INVALID", "Invalid legacy account summary")
	}
	a := AccountUsage{Summary: &AccountSummary{old.Lifetime, old.Peak, old.Turn, old.Current, old.Longest}, DailyUsageBuckets: []AccountDay{}}
	for _, b := range days {
		var d struct {
			Date   string `json:"start_date"`
			Tokens int64  `json:"tokens"`
		}
		if json.Unmarshal(b, &d) != nil {
			return a, fault.New("LEGACY_ROW_INVALID", "Invalid legacy account day")
		}
		a.DailyUsageBuckets = append(a.DailyUsageBuckets, AccountDay{d.Date, d.Tokens})
	}
	b, _ := json.Marshal(a)
	return ParseAccountUsage(b)
}
