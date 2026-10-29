package desktop

import (
	"image/color"
	"testing"

	"github.com/egoist/mygo/ui"
	"lam.local/lam2/internal/model"
	"lam.local/lam2/internal/usage"
)

func TestNativeStylesLightAndDarkHaveIndependentTokens(t *testing.T) {
	light, dark := paletteFor(false), paletteFor(true)
	if light.Dark || !dark.Dark {
		t.Fatal("appearance must track system mode")
	}
	if light.Canvas.R == dark.Canvas.R && light.Canvas.G == dark.Canvas.G {
		t.Fatal("light and dark windows have identical canvas")
	}
	if light.Surface.R == light.Canvas.R && light.Surface.G == light.Canvas.G {
		t.Fatal("cards must contrast with canvas")
	}
	if dark.Surface.R == dark.Canvas.R && dark.Surface.G == dark.Canvas.G {
		t.Fatal("dark cards must contrast with canvas")
	}
	if light.Accent.R == light.Border.R && light.Accent.G == light.Border.G {
		t.Fatal("interactive accent missing")
	}
}

func TestOriginalFivePageVisualHierarchy(t *testing.T) {
	u := New(nil)
	u.locale = "en"
	for _, tc := range []struct {
		path   string
		labels []string
	}{
		{"/overview", []string{"Accounts", "Providers", "Quota usable", "Active source", "Codex", "Antigravity"}},
		{"/usage", []string{"Data & filters", "TOKEN VOLUMES", "PRODUCTIVITY & STREAKS", "COST & CREDITS"}},
		{"/sessions", []string{"Session library", "Sessions"}},
		{"/providers", []string{"Provider catalog", "Providers"}},
		{"/settings", []string{"General Settings", "APP SETTINGS", "▥  Rate Card", "▣  System & Desktop"}},
	} {
		u.router.Push(tc.path)
		if tc.path == "/usage" {
			u.usageFiltersOpen = true
		}
		tt := ui.NewTester(u.view, 1350, 940)
		for _, label := range tc.labels {
			if !tt.HasText(label) {
				t.Errorf("%s lost styled area %q: %v", tc.path, label, tt.Texts())
			}
		}
	}
}

func TestOverviewAccountCardSelectsExistingOperations(t *testing.T) {
	u := New(nil)
	u.locale = "en"
	u.basic.Profiles = []model.Profile{{ID: "main", Name: "main", AuthMode: "profile", Home: "/tmp/example-codex"}}
	tt := ui.NewTester(u.view, 1280, 900)
	if !tt.HasText("▷ Relay Latest") || !tt.HasText("▷ Handoff") {
		t.Fatal("account card lost its two actions")
	}
	if err := tt.Click("⋮"); err != nil {
		t.Fatal(err)
	}
	if u.profileRow != 0 {
		t.Fatalf("manage should select correct account, got %d", u.profileRow)
	}
}

func TestMyGoRenderDrawsTitlebarCanvasAndDock(t *testing.T) {
	u := New(nil)
	u.locale = "en"
	// ui.Render executes the actual native layout/paint pipeline, not a mock
	// CSS/HTML render. A flat, unstyled view would leave these both equal.
	im := ui.Render(u.view, 1280, 850, 1)
	if im == nil || im.Bounds().Dx() != 1280 || im.Bounds().Dy() != 850 {
		t.Fatal("invalid native render")
	}
	header := color.RGBAModel.Convert(im.At(4, 8)).(color.RGBA)
	canvas := color.RGBAModel.Convert(im.At(4, 180)).(color.RGBA)
	if header.R == canvas.R && header.G == canvas.G && header.B == canvas.B {
		t.Fatal("titlebar and canvas should have contrasting native fills")
	}
}

// These checks are separate from business service tests: they exercise the
// native styling, theme preference and presentation pathways of actual MyGo.
func TestOriginalOverviewAgentTabsAndSourceBar(t *testing.T) {
	u := New(nil)
	u.locale = "en"
	tt := ui.NewTester(u.view, 1280, 900)
	for _, label := range []string{"Codex", "Antigravity", "Active source", "Quota usable"} {
		if !tt.HasText(label) {
			t.Fatalf("overview missing original visual affordance %q", label)
		}
	}
	if err := tt.Click("Antigravity"); err != nil {
		t.Fatal(err)
	}
	if !tt.HasText("No Antigravity quota snapshot; refresh to query the local process") {
		t.Fatal("Antigravity tab must render a no-data native card, never a fabricated quota")
	}
}

func TestTrayVisualCardsAcrossSystemTheme(t *testing.T) {
	u := New(nil)
	u.locale = "en"
	tt := ui.NewTester(u.trayView, 520, 740)
	tt.SetDark(true)
	for _, label := range []string{"LAM", "Accounts", "Codex", "Antigravity"} {
		if !tt.HasText(label) {
			t.Fatalf("dark tray missing %q", label)
		}
	}
	tt.SetDark(false)
	if !tt.HasText("Accounts") {
		t.Fatal("light tray lost visual summary")
	}
}

func TestUsageNativeActivityHeatmapHasHorizontalLegend(t *testing.T) {
	u := New(nil)
	u.locale = "en"
	u.router.Push("/usage")
	u.usage.Daily = []usage.Daily{{Date: "2026-10-28", Tokens: 1234, Records: 4}}
	tt := ui.NewTester(u.view, 1280, 900)
	for _, label := range []string{"Token activity", "Activity level", "Aggregation", "Measure"} {
		if !tt.HasText(label) {
			t.Fatalf("usage heatmap missing %q", label)
		}
	}
}

func TestOriginalHeatmapPaletteLightAndDark(t *testing.T) {
	light, dark := heatmapPalette(false), heatmapPalette(true)
	for _, tc := range []struct {
		p    [5]ui.Color
		want [5]string
	}{
		{light, [5]string{"#ebedf0", "#fbcfe8", "#f472b6", "#ec4899", "#db2777"}},
		{dark, [5]string{"#1e293b", "#4a153b", "#7c1a60", "#ae1e88", "#ec4899"}},
	} {
		for i, hex := range tc.want {
			expected := ui.Hex(hex)
			if tc.p[i].R != expected.R || tc.p[i].G != expected.G || tc.p[i].B != expected.B {
				t.Fatalf("heatmap token %d differs from original %s", i, hex)
			}
		}
	}
}
