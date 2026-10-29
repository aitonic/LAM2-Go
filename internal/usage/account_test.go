package usage

import (
	"testing"
	"time"
)

func TestAccountUsageKeepsNullsAndComparesOnlyKnownDays(t *testing.T) {
	a, e := ParseAccountUsage([]byte(`{"summary":{"lifetimeTokens":1000,"peakDailyTokens":null},"dailyUsageBuckets":[{"startDate":"2026-10-01","tokens":120},{"startDate":"2026-10-02","tokens":30}]}`))
	if e != nil {
		t.Fatal(e)
	}
	if a.Summary.PeakDailyTokens != nil || a.Summary.LifetimeTokens == nil {
		t.Fatal(a)
	}
	after := time.Date(2026, 10, 1, 0, 0, 0, 0, time.UTC)
	before := after.AddDate(0, 0, 2)
	d := CompareAccountDays(a, []Daily{{Date: "2026-10-01", Tokens: 100}, {Date: "2026-10-02", Tokens: 20}, {Date: "2026-10-03", Tokens: 999}}, after, before, time.UTC)
	if !d.Comparable || d.AccountTokens != 150 || d.LocalTokens != 120 || d.Delta != 30 {
		t.Fatal(d)
	}
	missing, _ := ParseAccountUsage([]byte(`{"summary":null,"dailyUsageBuckets":null}`))
	if CompareAccountDays(missing, nil, after, before, time.UTC).Comparable {
		t.Fatal("invented remote daily usage")
	}
	if _, e = ParseAccountUsage([]byte(`{"dailyUsageBuckets":[{"startDate":"x","tokens":-1}]}`)); e == nil {
		t.Fatal("invalid data accepted")
	}
}
