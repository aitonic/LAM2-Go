package desktop

import "github.com/egoist/mygo/ui"

// trayStyle mirrors the original trayPopoverPanel theme-specific CSS custom
// properties. It is a set of product design tokens, not another UI framework.
// Values are derived from LAM 0.4.4 src/styles.css lines 1516-1628.
type trayStyle struct {
	HeadBorder, HeadBG, StatsBorder, StatsBG, StatsDivider, StatsLabel ui.Color
	StatsValue, StatsAverage, StatsUsable                              ui.Color
	ActiveBorder, ActiveBG, ActiveText, ActiveValue                    ui.Color
	FooterBorder, FooterBG, PanelBorder, PanelShadow                   ui.Color
}

func originalTrayStyle(dark bool) trayStyle {
	if dark {
		return trayStyle{
			HeadBorder: ui.RGBA(255, 255, 255, 0.08), HeadBG: ui.RGBA(255, 255, 255, 0.015),
			StatsBorder: ui.RGBA(255, 255, 255, 0.08), StatsBG: ui.RGBA(255, 255, 255, 0.025),
			StatsDivider: ui.RGBA(255, 255, 255, 0.1), StatsLabel: ui.Hex("#94a3b8"),
			StatsValue: ui.Hex("#38bdf8"), StatsAverage: ui.Hex("#a78bfa"), StatsUsable: ui.Hex("#34d399"),
			ActiveBorder: ui.RGBA(255, 255, 255, 0.06), ActiveBG: ui.RGBA(255, 255, 255, 0.015),
			ActiveText: ui.Hex("#94a3b8"), ActiveValue: ui.Hex("#f1f5f9"),
			FooterBorder: ui.RGBA(255, 255, 255, 0.08), FooterBG: ui.RGBA(255, 255, 255, 0.02),
			PanelBorder: ui.RGBA(255, 255, 255, 0.1), PanelShadow: ui.RGBA(0, 0, 0, 0.5),
		}
	}
	return trayStyle{
		HeadBorder: ui.RGBA(15, 23, 42, 0.08), HeadBG: ui.RGBA(15, 23, 42, 0.01),
		StatsBorder: ui.RGBA(15, 23, 42, 0.08), StatsBG: ui.RGBA(15, 23, 42, 0.025),
		StatsDivider: ui.RGBA(15, 23, 42, 0.08), StatsLabel: ui.Hex("#64748b"),
		StatsValue: ui.Hex("#2563eb"), StatsAverage: ui.Hex("#7c3aed"), StatsUsable: ui.Hex("#059669"),
		ActiveBorder: ui.RGBA(15, 23, 42, 0.05), ActiveBG: ui.RGBA(15, 23, 42, 0.01),
		ActiveText: ui.Hex("#64748b"), ActiveValue: ui.Hex("#0f172a"),
		FooterBorder: ui.RGBA(15, 23, 42, 0.08), FooterBG: ui.RGBA(15, 23, 42, 0.02),
		PanelBorder: ui.RGBA(15, 23, 42, 0.08), PanelShadow: ui.RGBA(15, 23, 42, 0.12),
	}
}
