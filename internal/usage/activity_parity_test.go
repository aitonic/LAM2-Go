package usage

import (
	"testing"
	"time"
)

func TestLegacyHeatmapCallsTokensDailyWeeklyCumulative(t *testing.T) {
	days := []Daily{
		{Date: "2026-10-24", Records: 2, Tokens: 20}, // Saturday
		{Date: "2026-10-25", Records: 3, Tokens: 30}, // Sunday
		{Date: "2026-10-26", Records: 4, Tokens: 40}, // Monday
	}
	cases := []struct {
		metric, mode string
		values       []int64
	}{
		{"calls", "daily", []int64{2, 3, 4}},
		{"calls", "weekly", []int64{2, 7, 7}}, // Sunday-to-Saturday week
		{"calls", "cumulative", []int64{2, 5, 9}},
		{"tokens", "daily", []int64{20, 30, 40}},
		{"tokens", "weekly", []int64{20, 70, 70}},
		{"tokens", "cumulative", []int64{20, 50, 90}},
	}
	for _, tc := range cases {
		values, err := HeatmapValues(days, tc.metric, tc.mode)
		if err != nil || len(values) != len(tc.values) {
			t.Fatalf("%s/%s: %v %v", tc.metric, tc.mode, values, err)
		}
		for i, want := range tc.values {
			if values[i] != want {
				t.Fatalf("%s/%s[%d]: got %d want %d", tc.metric, tc.mode, i, values[i], want)
			}
		}
	}
	if _, err := HeatmapValues(days, "money", "weekly"); err == nil {
		t.Fatal("invalid metric")
	}
	if _, err := HeatmapValues(days, "calls", "sum"); err == nil {
		t.Fatal("invalid mode")
	}
	end := time.Date(2026, 10, 28, 0, 0, 0, 0, time.UTC)
	if first := DefaultActivityStart(end); first.Format("2006-01-02") != "2025-10-29" {
		t.Fatal("activity must cover 365 UTC days", first)
	}
}
