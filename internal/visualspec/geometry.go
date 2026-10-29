// Package visualspec records dimensions from the original LAM 0.4.4
// stylesheet. This is a reference contract, not a second UI framework.
package visualspec

const (
	OriginalNarrowBreakpoint  = 1240
	OriginalMobileBreakpoint  = 640
	NarrowTitlebarPadY        = 12
	NarrowTitlebarPadX        = 16
	UsagePagePadding          = 18
	UsagePageGap              = 14
	UsageHeaderTitleSize      = 22
	UsageHeaderSubtitleSize   = 13
	UsageHeaderLogoSize       = 34
	UsageHeaderLogoRadius     = 8
	UsageHeaderIconSize       = 20
	HeatmapCellSize           = 14
	HeatmapCellGap            = 3
	SessionsTableMinWidth     = 1280
	UsageHeaderControlsGap    = 12
	LegacyScrollBottomSpace   = 120
	NarrowDockIcon            = 28
	NarrowDockIconRadius      = 8
	NarrowDockLabelSize       = 9
	NarrowDockLabelWidth      = 48
	NarrowDockGap             = 6
	NarrowDockItemPadY        = 4
	NarrowDockItemPadX        = 5
	NarrowDockPaddingX        = 12
	NarrowDockRadius          = 14
	NarrowDockBottomGap       = 8
	RootFontSize              = 14
	RootLineHeight            = 1.45
	CardPadding               = 16
	CardGap                   = 10
	CardRadius                = 14
	OverviewStatValueFontSize = 26
	OverviewStatValueGap      = 2
	TitlebarActionsGap        = 14
	DockMaxWidth              = 920
	DockRadius                = 18
	DockBlur                  = 28
	DockAlphaLight            = .38
	DockAlphaDark             = .48
	UsageKpiHeaderSize        = 11.5
	UsageKpiMainSize          = 30
	UsageKpiIconSize          = 30
	UsageKpiBodyGap           = 16
	SettingsSidebarGap        = 16
	SettingsContentPadTop     = 32
	SettingsContentPadX       = 40
	SettingsContentPadBottom  = 140
	SettingsRowPadY           = 16
	SettingsRowPadX           = 20
	SettingsRowGap            = 24
	SettingsRowTitleSize      = 13.5
	SettingsRowDescSize       = 12
	TitlebarHeight            = 56
	StatusbarHeight           = 26
	StatusbarSidePadding      = 16
	PageSidePadding           = 24
	PageTopPadding            = 20
	FloatingDockBottomGap     = 10 // above statusbar
	FloatingDockIcon          = 32
	FloatingDockIconRadius    = 9
	FloatingDockLabelSize     = 10
	FloatingDockItemWidth     = 56
	FloatingDockGap           = 10
	FloatingDockPaddingX      = 14
	FloatingDockPaddingY      = 6
	OverviewStatCount         = 4
	OverviewStatIcon          = 42
	OverviewStatIconRadius    = 12
	OverviewStatGap           = 14
	OverviewStatRadius        = 14
	OverviewStatCardGap       = 12
	AccountMinWidth           = 280
	AccountGap                = 10
	AccountCardPadding        = 16
	AccountCardRadius         = 14
	UsageKpiCount             = 3
	UsageKpiGap               = 16
	UsageKpiPadding           = 20
	UsageKpiAccentHeight      = 3
	UsageKpiRadius            = 16
	SettingsSidebarWidth      = 220
	SettingsSidebarPaddingY   = 24
	SettingsSidebarPaddingX   = 16
	MainWindowWidth           = 1280
	MainWindowHeight          = 820
	MainWindowMinWidth        = 1100
	MainWindowMinHeight       = 700
	TrayWidth                 = 328
	TrayHeight                = 520
)

// AccountColumns mirrors CSS grid repeat(auto-fill, minmax(280px, 1fr)).
// The input is the available content width after its page-side padding.
func AccountColumns(contentWidth int) int {
	// The original 1240px media query forces .accountCardGrid to one column,
	// even when the auto-fill formula would otherwise fit three or four.
	if contentWidth+2*PageSidePadding <= OriginalNarrowBreakpoint {
		return 1
	}
	if contentWidth <= AccountMinWidth {
		return 1
	}
	count := (contentWidth + AccountGap) / (AccountMinWidth + AccountGap)
	if count < 1 {
		return 1
	}
	return count
}

// DockWidth mirrors the original five icons with their 10px gaps.
func DockWidth(items int) int {
	if items < 1 {
		return 0
	}
	return 2*FloatingDockPaddingX + items*FloatingDockItemWidth + (items-1)*FloatingDockGap
}

// SessionColumnWidths preserves the legacy .sessionCol* percentages (22/7/13/16/12/30)
// and its 1280px minimum layout width. They are native table column minima,
// not proof that the platform's actual cell painting matches WebKit.
func SessionColumnWidths(available int) [6]int {
	if available < SessionsTableMinWidth {
		available = SessionsTableMinWidth
	}
	pct := [6]int{22, 7, 13, 16, 12, 30}
	var widths [6]int
	remaining := available
	for i := 0; i < len(widths)-1; i++ {
		widths[i] = (available*pct[i] + 50) / 100
		remaining -= widths[i]
	}
	widths[5] = remaining
	return widths
}
