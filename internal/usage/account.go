package usage

import (
	"bytes"
	"encoding/json"
	"io"
	"lam.local/lam2/internal/fault"
	"math"
	"sort"
	"time"
)

// AccountUsage is an upstream account view, not a bill or a local-session total.
// Nil metrics/buckets remain unknown instead of being presented as zero.
type AccountSummary struct {
	LifetimeTokens        *int64 `json:"lifetimeTokens"`
	PeakDailyTokens       *int64 `json:"peakDailyTokens"`
	LongestRunningTurnSec *int64 `json:"longestRunningTurnSec"`
	CurrentStreakDays     *int64 `json:"currentStreakDays"`
	LongestStreakDays     *int64 `json:"longestStreakDays"`
}
type AccountDay struct {
	StartDate string `json:"startDate"`
	Tokens    int64  `json:"tokens"`
}
type AccountUsage struct {
	Summary           *AccountSummary `json:"summary"`
	DailyUsageBuckets []AccountDay    `json:"dailyUsageBuckets"`
}
type AccountDelta struct {
	Comparable                        bool
	AccountTokens, LocalTokens, Delta int64
	Days                              int
	Reason                            string
}

func ParseAccountUsage(raw []byte) (AccountUsage, error) {
	var a AccountUsage
	d := json.NewDecoder(bytes.NewReader(raw))
	if e := d.Decode(&a); e != nil {
		return a, fault.New("USAGE_INVALID", "Account usage is not a valid response")
	}
	var extra any
	if d.Decode(&extra) != io.EOF {
		return a, fault.New("USAGE_INVALID", "Account usage has trailing data")
	}
	if a.Summary != nil {
		for _, n := range []*int64{a.Summary.LifetimeTokens, a.Summary.PeakDailyTokens, a.Summary.LongestRunningTurnSec, a.Summary.CurrentStreakDays, a.Summary.LongestStreakDays} {
			if n != nil && *n < 0 {
				return a, fault.New("USAGE_INVALID", "Account usage contains a negative metric")
			}
		}
	}
	seen := map[string]bool{}
	for _, b := range a.DailyUsageBuckets {
		t, e := time.Parse("2006-01-02", b.StartDate)
		if e != nil || t.Format("2006-01-02") != b.StartDate || b.Tokens < 0 || seen[b.StartDate] {
			return a, fault.New("USAGE_INVALID", "Account usage contains an invalid or duplicate day")
		}
		seen[b.StartDate] = true
	}
	sort.Slice(a.DailyUsageBuckets, func(i, j int) bool { return a.DailyUsageBuckets[i].StartDate < a.DailyUsageBuckets[j].StartDate })
	return a, nil
}

// CompareAccountDays compares only explicitly reported complete UTC days.
// No inference is made about absent remote days or the lifetime metric. The
// caller must have already selected one identical Profile/account scope.
func CompareAccountDays(a AccountUsage, local []Daily, after, before time.Time, loc *time.Location) AccountDelta {
	d := AccountDelta{}
	if loc == nil {
		loc = time.UTC
	}
	if a.DailyUsageBuckets == nil {
		d.Reason = "Upstream daily usage is unavailable"
		return d
	}
	if after.IsZero() || before.IsZero() || !after.Before(before) {
		d.Reason = "A bounded date range is required"
		return d
	}
	// The API supplies date labels, not a timezone declaration. UTC is the
	// conservative comparison convention; local-time totals remain separate.
	_, aoff := after.In(loc).Zone()
	_, boff := before.Add(-time.Nanosecond).In(loc).Zone()
	if aoff != 0 || boff != 0 {
		d.Reason = "Daily timezone alignment is unverified; use UTC to compare"
		return d
	}
	byDay := map[string]int64{}
	for _, v := range local {
		if v.Tokens < 0 || v.Tokens > math.MaxInt64-byDay[v.Date] {
			d.Reason = "Invalid local daily total"
			return d
		}
		byDay[v.Date] += v.Tokens
	}
	for _, b := range a.DailyUsageBuckets {
		t, e := time.ParseInLocation("2006-01-02", b.StartDate, loc)
		if e != nil {
			continue
		}
		if t.Before(after) || t.AddDate(0, 0, 1).After(before) {
			continue
		}
		if b.Tokens > math.MaxInt64-d.AccountTokens || byDay[b.StartDate] > math.MaxInt64-d.LocalTokens {
			d.Reason = "Daily total exceeds the supported range"
			return AccountDelta{Reason: d.Reason}
		}
		d.AccountTokens += b.Tokens
		d.LocalTokens += byDay[b.StartDate]
		d.Days++
	}
	if d.Days == 0 {
		d.Reason = "No complete days are reported in both requested scopes"
		return d
	}
	d.Comparable = true
	d.Delta = d.AccountTokens - d.LocalTokens
	return d
}
