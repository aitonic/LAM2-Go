package desktop

import (
	"fmt"
	"os"
	"strings"

	"github.com/egoist/mygo/ui"
	"github.com/egoist/mygo/ui/chart"
	"lam.local/lam2/internal/visualspec"
)

// Palette translates the current system appearance into LAM's original
// dashboard visual language. These are native MyGo element styles, not CSS.
// Keep the light colors paired with the old LAM design tokens; dark colors
// remain explicit rather than placing a pale card over the dark desktop.
type palette struct {
	Dark                                                     bool
	Canvas, Surface, SurfaceAlt, Border, Ink, Muted, Faint   ui.Color
	Accent, AccentSoft, Purple, PurpleSoft, Green, GreenSoft ui.Color
	Amber, AmberSoft, Danger, DangerSoft, Pink               ui.Color
	Shadow                                                   ui.Color
}

func paletteFor(dark bool) palette {
	if dark {
		return palette{
			Dark:   true,
			Canvas: ui.Hex("#0b0d12"), Surface: ui.Hex("#141820"), SurfaceAlt: ui.Hex("#1a2030"),
			Border: ui.RGBA(255, 255, 255, 0.10), Ink: ui.Hex("#eef2ff"), Muted: ui.Hex("#94a3b8"), Faint: ui.Hex("#64748b"),
			Accent: ui.Hex("#3b82f6"), AccentSoft: ui.RGBA(59, 130, 246, 0.15),
			Purple: ui.Hex("#7c3aed"), PurpleSoft: ui.RGBA(124, 58, 237, 0.15),
			Green: ui.Hex("#16a34a"), GreenSoft: ui.RGBA(22, 163, 74, 0.15),
			Amber: ui.Hex("#b45309"), AmberSoft: ui.RGBA(180, 83, 9, 0.15),
			Danger: ui.Hex("#dc2626"), DangerSoft: ui.Hex("#fef2f2"),
			Pink: ui.Hex("#ec4899"), Shadow: ui.RGBA(0, 0, 0, 0.35),
		}
	}
	return palette{
		Canvas: ui.Hex("#eef0f4"), Surface: ui.Hex("#ffffff"), SurfaceAlt: ui.Hex("#f5f6f8"),
		Border: ui.RGBA(15, 23, 42, 0.10), Ink: ui.Hex("#0f172a"), Muted: ui.Hex("#64748b"), Faint: ui.Hex("#94a3b8"),
		Accent: ui.Hex("#2563eb"), AccentSoft: ui.Hex("#eff6ff"),
		Purple: ui.Hex("#7c3aed"), PurpleSoft: ui.Hex("#f5f3ff"),
		Green: ui.Hex("#16a34a"), GreenSoft: ui.Hex("#ecfdf3"),
		Amber: ui.Hex("#b45309"), AmberSoft: ui.Hex("#fffbeb"),
		Danger: ui.Hex("#dc2626"), DangerSoft: ui.Hex("#fef2f2"),
		Pink: ui.Hex("#db2777"), Shadow: ui.RGBA(15, 23, 42, 0.06),
	}
}

// A theme copy changes *all* built-in widgets, including fields, tables,
// badges and focus rings. One function at the beginning of each window view
// is the only place that changes the MyGo theme.
func applyLAMTheme(c *ui.Context) palette {
	p := paletteFor(c.Theme().Dark)
	t := *c.Theme()
	t.Accent = p.Accent
	t.Border = p.Border
	t.Text = p.Ink
	t.TextMuted = p.Muted
	t.Background = p.Canvas
	t.Surface = p.Surface
	t.Radius = 10
	t.Spacing = 4
	t.FontSize = visualspec.RootFontSize
	// Calling SetTheme unconditionally during every build can generate
	// redundant invalidations. Only update when a token actually differs.
	current := c.Theme()
	if current.Accent.R != p.Accent.R || current.Accent.G != p.Accent.G || current.Accent.B != p.Accent.B ||
		current.Background.R != p.Canvas.R || current.Surface.R != p.Surface.R ||
		current.Radius != t.Radius || current.Spacing != t.Spacing || current.FontSize != t.FontSize {
		c.SetTheme(&t)
	}
	return p
}

func card(c *ui.Context, p palette) ui.Element {
	return ui.Column(c).Padding(visualspec.CardPadding).Gap(visualspec.CardGap).Radius(visualspec.CardRadius).
		Background(p.Surface).Border(1, p.Border).
		Shadow(0, 1, 2, 0, shadowSm(p))
}
func softCard(c *ui.Context, p palette) ui.Element {
	return ui.Column(c).Padding(13).Gap(9).Radius(11).
		Background(p.SurfaceAlt).Border(1, p.Border)
}
func cardHeader(c *ui.Context, p palette, title, subtitle string) {
	ui.Column(c).Gap(3).Children(func() {
		ui.Text(c, title).Bold().FontSize(16).TextColor(p.Ink)
		if subtitle != "" {
			ui.Text(c, subtitle).FontSize(12).TextColor(p.Muted)
		}
	})
}
func sectionHeading(c *ui.Context, p palette, title, subtitle string) {
	ui.Column(c).Gap(4).Children(func() {
		ui.Text(c, title).Bold().FontSize(17).TextColor(p.Ink)
		if subtitle != "" {
			ui.Text(c, subtitle).FontSize(12).TextColor(p.Muted)
		}
	})
}
func pill(c *ui.Context, p palette, label string, fg, bg ui.Color) {
	ui.Text(c, label).FontSize(11).Bold().TextColor(fg).
		Background(bg).Padding(5, 9).Radius(8)
}
func infoLine(c *ui.Context, p palette, value string) {
	ui.Text(c, value).FontSize(12).TextColor(p.Muted)
}
func metricCard(c *ui.Context, p palette, title, value string, accent, tint ui.Color) ui.Element {
	// Original .metricCard has no height declaration: its height comes from
	// the 42px icon, 16px vertical padding and intrinsic text line boxes.
	// Do not freeze this box to 85 DIP.
	card := ui.Row(c).Grow(1).
		Padding(16, 18).Gap(visualspec.OverviewStatGap).
		Radius(visualspec.OverviewStatRadius).Background(p.Surface).
		Border(1, p.Border).Shadow(0, 1, 2, 0, p.Shadow)
	card.Children(func() {
		icon := "accounts"
		switch title {
		case "Sessions":
			icon = "activity"
		case "Providers":
			icon = "providers"
		case "Quota usable":
			icon = "quota"
		}
		ui.Box(c).Size(visualspec.OverviewStatIcon, visualspec.OverviewStatIcon).Radius(visualspec.OverviewStatIconRadius).Background(tint).Center().Children(func() {
			originalIcon(c, icon, 20, accent)
		})
		ui.Column(c).Gap(visualspec.OverviewStatValueGap).Grow(1).Children(func() {
			ui.Text(c, title).FontSize(12).TextColor(p.Muted)
			ui.Text(c, value).Bold().FontSize(visualspec.OverviewStatValueFontSize).LetterSpacing(-0.02 * visualspec.OverviewStatValueFontSize).TextColor(p.Ink).FontFeatures("tnum")
		})
	})
	return card
}

func shortenPath(path string) string {
	if path == "" {
		return ""
	}
	if u, err := os.UserHomeDir(); err == nil && u != "" {
		if strings.HasPrefix(path, u) {
			return "~" + strings.TrimPrefix(path, u)
		}
	}
	return path
}

func planTypeBadge(c *ui.Context, p palette, planType string) {
	if planType == "" {
		return
	}
	typ := strings.ToLower(strings.TrimSpace(planType))
	label := strings.ToUpper(typ)

	badge := ui.Row(c).Height(18).Padding(0, 6).Radius(999).Center().Gap(3)
	switch typ {
	case "team":
		grad := ui.LinearGradient{Angle: 135, Stops: []ui.GradientStop{
			{Color: ui.Hex("#6366f1"), At: 0},
			{Color: ui.Hex("#3b82f6"), At: 1},
		}}
		badge.LinearGradient(grad).Children(func() {
			ui.Text(c, "👥").FontSize(9).TextColor(ui.Hex("#ffffff"))
			ui.Text(c, label).Bold().FontSize(9).TextColor(ui.Hex("#ffffff"))
		})
	case "pro":
		grad := ui.LinearGradient{Angle: 135, Stops: []ui.GradientStop{
			{Color: ui.Hex("#f97316"), At: 0},
			{Color: ui.Hex("#ea580c"), At: 1},
		}}
		badge.LinearGradient(grad).Children(func() {
			ui.Text(c, "⚡").FontSize(9).TextColor(ui.Hex("#ffffff"))
			ui.Text(c, label).Bold().FontSize(9).TextColor(ui.Hex("#ffffff"))
		})
	case "business":
		grad := ui.LinearGradient{Angle: 135, Stops: []ui.GradientStop{
			{Color: ui.Hex("#ec4899"), At: 0},
			{Color: ui.Hex("#8b5cf6"), At: 1},
		}}
		badge.LinearGradient(grad).Children(func() {
			ui.Text(c, label).Bold().FontSize(9).TextColor(ui.Hex("#ffffff"))
		})
	case "plus":
		badge.Background(ui.RGBA(22, 163, 74, 0.10)).Border(1, ui.RGBA(22, 163, 74, 0.25)).Children(func() {
			ui.Text(c, "+").Bold().FontSize(10).TextColor(ui.Hex("#16a34a"))
			ui.Text(c, label).Bold().FontSize(9).TextColor(ui.Hex("#16a34a"))
		})
	case "free":
		badge.Border(1, p.Border).Children(func() {
			ui.Text(c, label).Bold().FontSize(9).TextColor(p.Muted)
		})
	default:
		badge.Background(p.AccentSoft).Border(1, p.Accent.Alpha(0.25)).Children(func() {
			ui.Text(c, label).Bold().FontSize(9).TextColor(p.Accent)
		})
	}
}

func quotaWindowBar(c *ui.Context, p palette, label string, remaining *float64, resetText string) {
	hasData := remaining != nil
	remVal := 0.0
	if hasData {
		remVal = *remaining
		if remVal < 0 {
			remVal = 0
		} else if remVal > 100 {
			remVal = 100
		}
	}

	state := "safe"
	if !hasData {
		state = "na"
	} else if remVal == 0 {
		state = "empty"
	} else if remVal < 25 {
		state = "danger"
	} else if remVal < 70 {
		state = "warn"
	}

	ui.Column(c).Gap(4).Children(func() {
		ui.Row(c).Children(func() {
			ui.Text(c, label).FontSize(12).TextColor(p.Muted).Grow(1)
			valStr := "N/A"
			if hasData {
				valStr = fmt.Sprintf("%.0f%% left", remVal)
			}
			ui.Text(c, valStr).Bold().FontSize(13).TextColor(p.Ink)
		})

		slotBg := p.SurfaceAlt
		switch state {
		case "safe":
			slotBg = ui.RGBA(22, 163, 74, 0.12)
		case "warn":
			slotBg = ui.RGBA(245, 158, 11, 0.14)
		case "danger":
			slotBg = ui.RGBA(220, 38, 38, 0.12)
		default:
			slotBg = p.Border
		}

		barSlot := ui.Box(c).Height(10).Radius(5).Background(slotBg)
		if hasData && state != "empty" {
			barSlot.Draw(func(painter *ui.Painter, r ui.Rect) {
				w := r.W * float32(remVal/100.0)
				if w < 4 {
					w = 4
				}
				if w > r.W {
					w = r.W
				}
				var grad ui.LinearGradient
				switch state {
				case "safe":
					grad = ui.LinearGradient{Angle: 90, Stops: []ui.GradientStop{
						{Color: ui.Hex("#15803d"), At: 0},
						{Color: ui.Hex("#16a34a"), At: 0.55},
						{Color: ui.Hex("#22c55e"), At: 1},
					}}
				case "warn":
					grad = ui.LinearGradient{Angle: 90, Stops: []ui.GradientStop{
						{Color: ui.Hex("#ca8a04"), At: 0},
						{Color: ui.Hex("#f59e0b"), At: 0.55},
						{Color: ui.Hex("#fbbf24"), At: 1},
					}}
				case "danger":
					grad = ui.LinearGradient{Angle: 90, Stops: []ui.GradientStop{
						{Color: ui.Hex("#b91c1c"), At: 0},
						{Color: ui.Hex("#dc2626"), At: 0.55},
						{Color: ui.Hex("#ef4444"), At: 1},
					}}
				}
				painter.FillGradient(ui.Rect{X: r.X, Y: r.Y, W: w, H: r.H}, grad, r.H/2)
			})
		}

		if resetText != "" {
			ui.Text(c, resetText).FontSize(11).TextColor(p.Faint)
		} else if !hasData {
			ui.Text(c, "No quota data").FontSize(11).TextColor(p.Faint)
		}
	})
}
func errorBanner(c *ui.Context, p palette, message string) {
	ui.Row(c).Padding(12).Gap(8).Radius(10).
		Background(p.DangerSoft).Border(1, p.Danger.Alpha(.3)).Children(func() {
		ui.Text(c, "!").Bold().TextColor(p.Danger)
		ui.Text(c, message).TextColor(p.Danger).Grow(1)
	})
}
func progressLabel(c *ui.Context, p palette, name string, percent *float64, reset string) {
	ui.Row(c).Gap(8).Children(func() {
		ui.Text(c, name).FontSize(12).TextColor(p.Muted).Grow(1)
		text := QuotaLabel(percent)
		if percent != nil {
			text = fmt.Sprintf("%.0f%% left", *percent)
		}
		ui.Text(c, text).Bold().FontSize(12).TextColor(p.Ink)
	})
	if percent != nil {
		f := *percent / 100
		if f < 0 {
			f = 0
		}
		if f > 1 {
			f = 1
		}
		// Use the built-in native progress control, styled by LAM's theme.
		ui.Progress(c, f).Height(8)
	}
	if reset != "" {
		infoLine(c, p, reset)
	}
}

// quotaRing uses MyGo's own chart primitives; it does not create a canvas or
// custom chart renderer. Missing percentages are shown as unknown, not zero.
func quotaRing(c *ui.Context, p palette, remaining *float64) {
	val := 0.0
	text := "?"
	if remaining != nil {
		val = *remaining
		if val < 0 {
			val = 0
		}
		if val > 100 {
			val = 100
		}
		text = fmt.Sprintf("%.0f%%", val)
	}
	slices := chart.Pie([]float64{val, 100 - val}, 0.02)
	ring := ui.Box(c).Size(76, 76).Label("Remaining quota: " + text)
	ring.Draw(func(painter *ui.Painter, r ui.Rect) {
		cx, cy := r.X+r.W/2, r.Y+r.H/2
		for i, s := range slices {
			col := p.Border
			if i == 0 {
				col = p.Green
			}
			painter.FillPath(chart.Sector(cx, cy, 26, 34, s), col)
		}
	})
	ring.Children(func() {
		ui.Text(c, text).Bold().FontSize(16).TextColor(p.Ink).
			Attach(ui.AnchorCenter, ui.AnchorCenter)
	})
}

// usageKPI maps the original .usageMetricCard parameters. Its layout is
// content-sized (CSS did not set height:255px). Color, spacing, shadows,
// header typography and gradient stops come from the original stylesheet.
// Text gradient clipping still needs native macOS screenshot verification.
type usageStat struct{ Label, Value string }

func usageKPI(c *ui.Context, p palette, title, headline string, index int, details []usageStat) {
	gradients := [3][2]ui.Color{
		{ui.Hex("#6366f1"), ui.Hex("#3b82f6")},
		{ui.Hex("#f59e0b"), ui.Hex("#ef4444")},
		{ui.Hex("#10b981"), ui.Hex("#059669")},
	}
	if index < 0 || index >= len(gradients) {
		index = 0
	}
	colors := gradients[index]
	iconTint := [3]ui.Color{
		ui.RGBA(99, 102, 241, 0.09),
		ui.RGBA(245, 158, 11, 0.09),
		ui.RGBA(16, 185, 129, 0.09),
	}[index]
	gradient := ui.LinearGradient{Angle: 90, Stops: []ui.GradientStop{{Color: colors[0], At: 0}, {Color: colors[1], At: 1}}}
	panel := ui.Column(c).Grow(1).Padding(visualspec.UsageKpiPadding).Gap(visualspec.UsageKpiBodyGap).
		Radius(visualspec.UsageKpiRadius).Background(p.Surface).Border(1, p.Border).
		Shadow(0, 4, 20, -2, ui.RGBA(0, 0, 0, 0.02)).Clip()
	// CSS .usageMetricCard:hover uses a distinct shadow and a -2px lift.
	if panel.Hovered() {
		panel.Shadow(0, 10, 30, -10, ui.RGBA(0, 0, 0, 0.08)).Transform(ui.Translation(0, -2))
	}
	panel.Children(func() {
		// The CSS ::before bar is positioned at the top edge, not below padding.
		ui.Box(c).Absolute().Top(0).Left(0).Right(0).
			Height(visualspec.UsageKpiAccentHeight).LinearGradient(gradient)
		ui.Row(c).Gap(10).Children(func() {
			ui.Box(c).Size(visualspec.UsageKpiIconSize, visualspec.UsageKpiIconSize).
				Radius(8).Background(iconTint).Center().Children(func() {
				ui.Text(c, "▣").FontSize(15).TextColor(colors[0])
			})
			ui.Text(c, title).FontSize(visualspec.UsageKpiHeaderSize).Bold().
				LetterSpacing(.05 * visualspec.UsageKpiHeaderSize).TextColor(p.Muted)
		})
		ui.Column(c).Gap(4).Padding(0, 0, 12, 0).BorderWidth(0, 0, 1, 0).
			BorderColor(p.Border).BorderStyle(ui.BorderDashed).Children(func() {
			ui.Text(c, headline).FontSize(visualspec.UsageKpiMainSize).
				FontWeight(800).LetterSpacing(-.02 * visualspec.UsageKpiMainSize).
				TextColor(colors[0]).SingleLine()
		})
		ui.Grid(c).Columns(2).GapX(16).GapY(12).Children(func() {
			for _, s := range details {
				ui.Column(c).Gap(4).Children(func() {
					ui.Text(c, s.Label).Bold().FontSize(10).LetterSpacing(.5).TextColor(p.Muted)
					ui.Text(c, s.Value).Bold().FontSize(15).TextColor(p.Ink).SingleLine()
				})
			}
		})
	})
}

// shadowSm is CSS --shadow-sm in each theme; generic p.Shadow cannot be
// reused because several panels have different y/blur/spread parameters.
func shadowSm(p palette) ui.Color {
	if p.Dark {
		return ui.RGBA(0, 0, 0, .35)
	}
	return ui.RGBA(15, 23, 42, .06)
}

// Exact sRGB interpolation of CSS color-mix(in srgb, accent N%, --line).
// The alpha result is premultiplied according to CSS Color 5.
func accountCardBorder(p palette, hover bool) ui.Color {
	if p.Dark {
		if hover {
			return ui.RGBA(93, 152, 248, .388)
		}
		return ui.RGBA(110, 163, 248, .298)
	}
	if hover {
		return ui.RGBA(33, 86, 201, .388)
	}
	return ui.RGBA(29, 70, 162, .226)
}

func rootShadowSm(p palette) ui.Color {
	if p.Dark {
		return ui.RGBA(0, 0, 0, .25)
	}
	return ui.RGBA(15, 23, 42, .05)
}

func rootShadowMd(p palette) ui.Color {
	if p.Dark {
		return ui.RGBA(0, 0, 0, .35)
	}
	return ui.RGBA(15, 23, 42, .06)
}
