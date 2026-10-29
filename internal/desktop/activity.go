package desktop

import (
	"fmt"
	"math"
	"time"

	"github.com/egoist/mygo/ui"
	"lam.local/lam2/internal/usage"
	"lam.local/lam2/internal/visualspec"
)

// Native MyGo heatmap: weeks across, Sunday-Saturday down. LAM 0.4.x used
// this GitHub-like orientation, not a 7-column month-sized calendar.
func (u *UI) activityView(c *ui.Context) {
	p := paletteFor(c.Theme().Dark)
	colors := heatmapPalette(p.Dark)
	a := u.usage.Activity
	ui.Row(c).Gap(10).Wrap().Children(func() {
		sectionHeading(c, p, "Token activity", "Daily, weekly or cumulative activity across the last year")
		ui.Spacer(c).Grow(1)
		ui.Select(c, &u.activityMode, []string{"daily", "weekly", "cumulative"}).Label("Aggregation").Width(155)
		ui.Select(c, &u.activityMetric, []string{"calls", "tokens"}).Label("Measure").Width(135)
	})
	values, err := usage.HeatmapValues(u.usage.Daily, u.activityMetric, u.activityMode)
	if err != nil {
		errorBanner(c, p, "Activity unavailable: "+err.Error())
		return
	}
	totals := map[string]int64{}
	var totalCalls int64
	details := map[string]usage.Daily{}
	peak := int64(1)
	for i, d := range u.usage.Daily {
		if i >= len(values) {
			break
		}
		totals[d.Date] = values[i]
		totalCalls += d.Records
		details[d.Date] = d
		if values[i] > peak {
			peak = values[i]
		}
	}
	if len(u.usage.Daily) == 0 {
		infoLine(c, p, "No activity in this date range.")
		return
	}
	// The legend and tooltips use the source day's record, even when the
	// highlighted intensity is the aggregate of a week or year-to-date.
	ui.Row(c).Gap(16).Children(func() {
		// Each column is a week; the 7 short native buttons remain usable
		// with both pointer and keyboard. Keep the chart horizontally scrollable
		// on smaller windows rather than squeezing cells into unreadability.
		ui.ScrollHorizontal(c).Grow(1).Height(133).Gap(4).Children(func() {
			end := time.Now().UTC().Truncate(24 * time.Hour)
			if len(u.usage.Daily) > 0 {
				if parsed, e := time.Parse("2006-01-02", u.usage.Daily[len(u.usage.Daily)-1].Date); e == nil {
					end = parsed
				}
			}
			rangeStart := end.AddDate(0, 0, -363)
			begin := rangeStart.AddDate(0, 0, -int(rangeStart.Weekday()))
			ui.Row(c).Gap(visualspec.HeatmapCellGap).Children(func() {
				for week := 0; week < 53; week++ {
					weekStart := begin.AddDate(0, 0, week*7)
					if weekStart.After(end) {
						break
					}
					ui.Column(c).Gap(visualspec.HeatmapCellGap).Children(func() {
						for day := 0; day < 7; day++ {
							date := weekStart.AddDate(0, 0, day)
							if date.Before(rangeStart) || date.After(end) {
								// Legacy .usageHeatmapCell--empty is transparent
								// and not clickable outside its date range.
								ui.Box(c).Size(visualspec.HeatmapCellSize, visualspec.HeatmapCellSize)
								continue
							}
							value, exists := totals[date.Format("2006-01-02")]
							d := details[date.Format("2006-01-02")]
							tone := colors[0]
							if exists && value > 0 {
								level := math.Sqrt(float64(value) / float64(peak))
								switch {
								case level < .23:
									tone = colors[1]
								case level < .48:
									tone = colors[2]
								case level < .75:
									tone = colors[3]
								default:
									tone = colors[4]
								}
							}
							label := fmt.Sprintf("%s · %s: %d · %d calls / %d tokens · USD %.4f estimated", date.Format("2006-01-02"), u.activityMetric, value, d.Records, d.Tokens, d.EstimatedUSD)
							button := ui.Button(c, "").Size(visualspec.HeatmapCellSize, visualspec.HeatmapCellSize).Padding(0).Radius(3).Background(tone).
								Border(0, p.Border).Label(label).Tooltip(label)
							if button.Hovered() {
								// Legacy .usageHeatmapCell:hover uses scale(1.3) and
								// a 0/0/8px shadow. The 150ms cubic-bezier still needs
								// verified MyGo frame-timeline equivalence.
								button.Scale(1.3, 1.3).Shadow(0, 0, 8, 0, ui.RGBA(0, 0, 0, .12))
							}
							if button.Clicked() && !date.After(end) {
								u.after = date.Format("2006-01-02")
								u.before = date.AddDate(0, 0, 1).Format("2006-01-02")
								u.usagePreset = "custom"
								u.filter.Offset = 0
								u.usageTabIndex = 1
								u.reload()
							}
						}
					})
				}
			})
		})
		softCard(c, p).Width(185).Children(func() {
			infoLine(c, p, "Activity level")
			ui.Row(c).Gap(4).Children(func() {
				for _, col := range colors {
					ui.Box(c).Size(14, 14).Radius(3).Background(col)
				}
			})
			infoLine(c, p, "Active days")
			ui.Text(c, fmt.Sprint(a.ActiveDays)).Bold().FontSize(17).TextColor(p.Ink)
			infoLine(c, p, "Range calls")
			ui.Text(c, fmt.Sprint(totalCalls)).Bold().FontSize(17).TextColor(p.Ink)
			infoLine(c, p, "Range tokens")
			ui.Text(c, fmt.Sprint(a.TotalTokens)).Bold().FontSize(17).TextColor(p.Ink)
		})
	})
}

// Original styles.css :root and :root[data-theme=dark] heatmap tokens.
// Avoid reusing SurfaceAlt or a light-mode palette for the dark heatmap.
func heatmapPalette(dark bool) [5]ui.Color {
	if dark {
		return [5]ui.Color{ui.Hex("#1e293b"), ui.Hex("#4a153b"), ui.Hex("#7c1a60"), ui.Hex("#ae1e88"), ui.Hex("#ec4899")}
	}
	return [5]ui.Color{ui.Hex("#ebedf0"), ui.Hex("#fbcfe8"), ui.Hex("#f472b6"), ui.Hex("#ec4899"), ui.Hex("#db2777")}
}
