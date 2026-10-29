package visualspec

import "testing"

func TestLAM044OriginalFixedGeometry(t *testing.T) {
	if TitlebarHeight != 56 || StatusbarHeight != 26 || MainWindowWidth != 1280 || MainWindowHeight != 820 || MainWindowMinWidth != 1100 || MainWindowMinHeight != 700 {
		t.Fatal("window chrome differs from original")
	}
	if FloatingDockIcon != 32 || FloatingDockLabelSize != 10 || FloatingDockBottomGap != 10 {
		t.Fatal("floating dock changed")
	}
	if SettingsSidebarWidth != 220 || OverviewStatIcon != 42 || AccountMinWidth != 280 {
		t.Fatal("panel and card metrics changed")
	}
	if TrayWidth != 328 || TrayHeight != 520 {
		t.Fatal("original tray default window dimensions changed")
	}
	if UsageKpiCount != 3 || UsageKpiAccentHeight != 3 {
		t.Fatal("usage overview geometry changed")
	}
}
func TestAccountGridMatchesOriginalAutofill(t *testing.T) {
	for _, tc := range []struct{ window, want int }{
		{1600, 5}, {1280, 4}, {1240, 1}, {1100, 1}, {1050, 1}, {900, 1}, {600, 1},
	} {
		got := AccountColumns(tc.window - 2*PageSidePadding)
		if got != tc.want {
			t.Errorf("window %d: got %d columns, want %d", tc.window, got, tc.want)
		}
	}
}
func TestDockIsSmallFloatingIsland(t *testing.T) {
	if width := DockWidth(5); width > 370 || width < 300 {
		t.Fatalf("dock width is %d; original is a compact island", width)
	}
}

// Original 0.4.4 selectors: .usageDashboardPage, .usageDashboardTitle,
// .usageDashboardLogo, .usageDashboardHeader h2/p and .content.
// These constants are source contracts; native render must be checked too.
func TestOriginalUsagePageDimensions(t *testing.T) {
	if UsagePagePadding != 18 || UsagePageGap != 14 {
		t.Fatalf("usage page geometry drifted: pad=%d gap=%d", UsagePagePadding, UsagePageGap)
	}
	if UsageHeaderTitleSize != 22 || UsageHeaderSubtitleSize != 13 || UsageHeaderLogoSize != 34 || UsageHeaderLogoRadius != 8 || UsageHeaderIconSize != 20 || UsageHeaderControlsGap != 12 {
		t.Fatal("usage header typography/icon geometry diverged from legacy CSS")
	}
	if LegacyScrollBottomSpace != 120 {
		t.Fatal("legacy .content scrolling end pad is 120px")
	}
}

func TestOriginalResponsiveTitlebarParameters(t *testing.T) {
	if OriginalNarrowBreakpoint != 1240 || NarrowTitlebarPadY != 12 || NarrowTitlebarPadX != 16 || TitlebarHeight != 56 {
		t.Fatal("LAM 0.4.4 titlebar geometry changed")
	}
}

func TestOriginalHeatmapCellGeometry(t *testing.T) {
	if HeatmapCellSize != 14 || HeatmapCellGap != 3 {
		t.Fatal("original heatmap was 14px cells with 3px gutters")
	}
}

// Original styles.css: .sessionsTable min-width:1280 and the six col rules.
func TestOriginalSessionsSixColumnsAndProportions(t *testing.T) {
	if SessionsTableMinWidth != 1280 {
		t.Fatal("original Sessions min-width changed")
	}
	for _, available := range []int{900, 1280, 1600} {
		widths := SessionColumnWidths(available)
		sum := 0
		for _, w := range widths {
			if w <= 0 {
				t.Fatal("empty column")
			}
			sum += w
		}
		want := available
		if want < 1280 {
			want = 1280
		}
		if sum != want {
			t.Fatalf("width=%d: column sum %d, want %d", available, sum, want)
		}
		if widths[0] != (want*22+50)/100 || widths[1] != (want*7+50)/100 || widths[2] != (want*13+50)/100 || widths[3] != (want*16+50)/100 || widths[4] != (want*12+50)/100 {
			t.Fatalf("column proportions changed: %v", widths)
		}
	}
}
