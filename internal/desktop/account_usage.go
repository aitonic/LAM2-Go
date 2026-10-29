package desktop

import (
	"context"
	"fmt"
	"github.com/egoist/mygo/ui"
	"lam.local/lam2/internal/usage"
	"time"
)

func optionalMetric(n *int64) string {
	if n == nil {
		return "Unknown"
	}
	return fmt.Sprint(*n)
}
func (u *UI) accountUsageView(c *ui.Context) {
	if len(u.usage.LegacyAccounts) > 0 {
		ui.Text(c, "Imported historical account snapshots have no verified account attribution.").FontSize(12)
		if ui.Button(c, "View preserved legacy account usage").Clicked() {
			u.info("Legacy account snapshots - not assigned to selected profile", u.usage.LegacyAccounts)
		}
	}

	ui.Row(c).Gap(8).Children(func() {
		if ui.Button(c, "Refresh account usage").Disabled(u.usageProfile == "" || u.busy).Clicked() {
			id := u.usageProfile
			u.task("Refresh account usage", func(ctx context.Context) (any, error) { return u.core.AccountUsage(ctx, id) }, func(any) { u.reload() })
		}
		ui.Text(c, "Select one profile. Cached data remains visible when a refresh fails.").FontSize(12)
	})
	if u.usageProfile == "" {
		ui.Text(c, "Select a profile to view account-reported usage.")
		return
	}
	a := u.usage.Account
	if a == nil {
		ui.Text(c, "No account usage snapshot yet. Refresh to request it from Codex.")
		return
	}
	ui.Text(c, "Fetched: "+u.usage.AccountFetchedAt.Local().Format("2006-01-02 15:04:05")).FontSize(12)
	if a.Summary != nil {
		s := a.Summary
		ui.Text(c, "Lifetime tokens: "+optionalMetric(s.LifetimeTokens)+" | Peak day: "+optionalMetric(s.PeakDailyTokens)).Bold()
		ui.Text(c, "Current streak: "+optionalMetric(s.CurrentStreakDays)+" | Longest streak: "+optionalMetric(s.LongestStreakDays)+" | Longest turn (sec): "+optionalMetric(s.LongestRunningTurnSec))
	} else {
		ui.Text(c, "Account summary unavailable")
	}
	d := u.usage.AccountDelta
	if d.Comparable {
		ui.Text(c, fmt.Sprintf("Matched %d UTC days: account %d / local %d / delta %+d tokens", d.Days, d.AccountTokens, d.LocalTokens, d.Delta))
	} else if d.Reason != "" {
		ui.Text(c, d.Reason).FontSize(12)
	}
	if a.DailyUsageBuckets == nil {
		ui.Text(c, "Daily usage is unknown, not zero.")
		return
	}
	days := []usage.Daily{}
	for _, b := range a.DailyUsageBuckets {
		if (u.after == "" || b.StartDate >= u.after) && (u.before == "" || b.StartDate < u.before) {
			days = append(days, usage.Daily{Date: b.StartDate, Tokens: b.Tokens})
		}
	}
	// Account days have no per-call costs or counts. Display them directly rather
	// than accidentally reusing a local chart with a different scope.
	ui.Scroll(c).Grow(1).Gap(5).Children(func() {
		ui.Grid(c).Columns(7).Gap(5).Children(func() {
			for _, b := range days {
				date, e := time.Parse("2006-01-02", b.Date)
				if e != nil {
					continue
				}
				ui.Text(c, fmt.Sprintf("%s\n%d", date.Format("01-02"), b.Tokens)).Padding(8).Background(c.Theme().Surface).Tooltip(b.Date + " / account-reported tokens")
			}
		})
		if len(days) == 0 {
			ui.Text(c, "No reported days in the selected range")
		}
	})
}
