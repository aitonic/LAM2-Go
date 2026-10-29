package desktop

import (
	"fmt"
	"math"
	"time"

	"github.com/egoist/mygo/ui"
	"lam.local/lam2/internal/usage"
)

// The heatmap uses native MyGo Grid and controls rather than HTML or custom
// retained drawing. Each cell is a source day, including in weekly mode.
func (u *UI) activityView(c *ui.Context) {
	a := u.usage.Activity
	ui.Scroll(c).Grow(1).Gap(10).Children(func() {
		ui.Text(c, fmt.Sprintf("Active days %d | Current streak %d | Longest streak %d", a.ActiveDays, a.CurrentStreak, a.LongestStreak)).Bold()
		ui.Text(c, fmt.Sprintf("Peak %s: %d tokens | Period total %d", a.Peak.Date, a.Peak.Tokens, a.TotalTokens))
		ui.Row(c).Gap(8).Children(func() {
			ui.Select(c, &u.activityMode, []string{"daily", "weekly", "cumulative"}).Label("Activity mode").Width(180)
			ui.Select(c, &u.activityMetric, []string{"calls", "tokens"}).Label("Activity metric").Width(160)
		})
		values, err := usage.HeatmapValues(u.usage.Daily, u.activityMetric, u.activityMode)
		if err != nil {
			ui.Text(c, "Activity display unavailable: "+err.Error())
			return
		}
		peak := int64(1)
		for _, v := range values {
			peak = max(peak, v)
		}
		ui.Grid(c).Columns(7).Gap(4).Children(func() {
			for _, weekday := range []string{"Sun", "Mon", "Tue", "Wed", "Thu", "Fri", "Sat"} {
				ui.Text(c, weekday).Bold()
			}
			if len(u.usage.Daily) > 0 {
				first, e := time.Parse("2006-01-02", u.usage.Daily[0].Date)
				if e == nil {
					for n := 0; n < int(first.Weekday()); n++ {
						ui.Box(c).Height(28)
					}
				}
			}
			for i, d := range u.usage.Daily {
				if i >= len(values) {
					break
				}
				date, e := time.Parse("2006-01-02", d.Date)
				if e != nil {
					continue
				}
				val := values[i]
				shade := c.Theme().Surface
				if val > 0 {
					strength := float32(.2 + .8*math.Sqrt(float64(val)/float64(peak)))
					shade = c.Theme().Accent.Alpha(strength)
				}
				if ui.Button(c, fmt.Sprint(date.Day())).Height(28).Background(shade).Tooltip(fmt.Sprintf("%s | %s %d %s | daily %d calls / %d tokens | USD %.6f estimate", d.Date, u.activityMode, val, u.activityMetric, d.Records, d.Tokens, d.EstimatedUSD)).Clicked() {
					u.after = d.Date
					u.before = date.AddDate(0, 0, 1).Format("2006-01-02")
					u.usagePreset = "custom"
					u.filter.Offset = 0
					u.usageTabIndex = 0
					u.reload()
				}
			}
		})
	})
}
