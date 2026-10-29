package usage

import (
	"sort"
	"time"

	"lam.local/lam2/internal/fault"
)

// DefaultActivityStart matches the original full-year activity panel. The
// summary itself is not limited by this charting window.
func DefaultActivityStart(end time.Time) time.Time {
	endDay := end.UTC().Truncate(24 * time.Hour)
	return endDay.AddDate(0, 0, -364)
}

// HeatmapValues retains one cell per day in all three display modes. The old
// weekly mode shows the Sunday-Saturday total on *each* day of that week.
func HeatmapValues(days []Daily, metric, mode string) ([]int64, error) {
	if metric != "calls" && metric != "tokens" {
		return nil, fault.New("HEATMAP_METRIC_INVALID", "Choose calls or tokens")
	}
	if mode != "daily" && mode != "weekly" && mode != "cumulative" {
		return nil, fault.New("HEATMAP_MODE_INVALID", "Choose daily, weekly or cumulative")
	}
	pick := func(d Daily) int64 {
		if metric == "calls" {
			return d.Records
		}
		return d.Tokens
	}
	totals := map[string]int64{}
	running := int64(0)
	dates := make([]Daily, len(days))
	copy(dates, days)
	sort.Slice(dates, func(i, j int) bool { return dates[i].Date < dates[j].Date })
	for _, d := range dates {
		t, e := time.Parse("2006-01-02", d.Date)
		if e != nil {
			return nil, fault.New("HEATMAP_DATE_INVALID", "Invalid activity date")
		}
		start := t.AddDate(0, 0, -int(t.Weekday())).Format("2006-01-02")
		totals[start] += pick(d)
	}
	cumulative := map[string]int64{}
	for _, d := range dates {
		running += pick(d)
		cumulative[d.Date] = running
	}
	out := make([]int64, len(days))
	for i, d := range days {
		t, e := time.Parse("2006-01-02", d.Date)
		if e != nil {
			return nil, fault.New("HEATMAP_DATE_INVALID", "Invalid activity date")
		}
		switch mode {
		case "daily":
			out[i] = pick(d)
		case "weekly":
			out[i] = totals[t.AddDate(0, 0, -int(t.Weekday())).Format("2006-01-02")]
		case "cumulative":
			out[i] = cumulative[d.Date]
		}
	}
	return out, nil
}
