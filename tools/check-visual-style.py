#!/usr/bin/env python3
"""Structural visual-parity gate. Never claims screenshot/native-render parity.

The original LAM 0.4.4 screenshot fixtures and source CSS are immutable;
checks link each major native element to its corresponding CSS dimensions.
A separate macOS screenshot comparison is REQUIRED for release acceptance.
"""
from pathlib import Path
import hashlib, json, sys, re
root = Path(__file__).resolve().parents[1]
d = root / 'internal' / 'desktop'
files = {name: (d / name).read_text() for name in ('design.go','view.go','overview.go','settings.go','activity.go','run.go','forms.go','brand.go','design_test.go','icons.go','background.go')}
geom = (root / 'internal/visualspec/geometry.go').read_text()
checks = {
    'Original theme and color tokens': ('design.go', ['"#eef0f4"','"#f5f6f8"','"#2563eb"','"#ffffff"','"#7c3aed"','"#16a34a"','"#0b0d12"']),
    'MyGo native theming, no CSS runtime': ('design.go', ['c.SetTheme(&t)','t.Accent','t.Radius','t.Spacing']),
    'Native styled cards and shadows': ('design.go',['func card(','Shadow(','Border(','Radius(']),
    'Exact original SVG icons via native MyGo ui.Icon': ('icons.go',['ui.MustParseSVG','ui.Icon(c, originalOverviewIcon)','ui.Icon(c, originalUsageIcon)']),
    'Original radial backgrounds via native MyGo SVG': ('background.go',['assets/bg/light.svg','assets/bg/dark.svg','ui.MustParseSVG','ui.Image(c, image)']),
    'Original tray icon asset and original window defaults': ('run.go',['Icon: originalTrayTemplate','MainWindowHeight','TrayWidth','TrayHeight']),
    'Real original Orbit logo bitmap': ('brand.go',['go:embed assets/lam-orbit-icon.png','ui.DecodeBitmap','ui.Image(c, lamOrbit)']),
    'Original titlebar, centered mode, theme toggle': ('view.go',['Height(visualspec.TitlebarHeight)','Grow(1).Center()', '"system"','"light"','"dark"','originalLAMLogo(c, 32)']),
    'True overlay floating dock': ('view.go',['.Absolute().Left(0).Right(0).Bottom(float32(bottomGap))','FloatingDockIcon', 'FloatingDockItemWidth','glass.Blur{Radius: visualspec.DockBlur}']),
    'Statusbar independent of dock': ('view.go',['Height(visualspec.StatusbarHeight)','"Operational · LAM"']),
    'Five original native routes': ('view.go',['"overview"','"usage"','"sessions"','"providers"','"settings"']),
    'Four small overview metrics without stripe': ('overview.go',['"Accounts"','"Sessions"','"Providers"','"Quota usable"','metricCard(']),
    'Account grid auto-fill and compact actions': ('overview.go',['AccountColumns(','"▷ Relay Latest"','"▷ Handoff"','ExpandedCardActions']),
    'No persistent overview account operations panel': ('overview.go',['ui.PopoverBase(c, anchor, &u.accountMenuOpen', 'u.accountOperations(c)']),
    'Usage original KPI title, three large cards': ('view.go',['"TOKEN VOLUMES"','"PRODUCTIVITY & STREAKS"','"COST & CREDITS"','usageKPI(']),
    'Usage four original insight tabs order': ('view.go',['"Insights", u.t("calls"), u.t("threads"), "Diagnostics"']),
    '53-week pink horizontal heatmap': ('activity.go',['weekStart','ui.ScrollHorizontal(c)','"#fbcfe8"','"#f472b6"','"#ec4899"']),
    'Settings flush left rail, 220 DIP': ('settings.go',['Width(visualspec.SettingsSidebarWidth)','"APP SETTINGS"','"INTEGRATIONS"','"RESOURCES"','settingsPreferenceRow']),
    'Tray segmented summary and footer': ('run.go',['func (u *UI) trayView(','"5h avg"','fmt.Sprintf("Codex  %d"','fmt.Sprintf("Antigravity  %d"','quotaRing(c, p','"⌁ Stats"','"× Quit"']),
    'Native modal styling': ('forms.go',['func (u *UI) dialogs(','Background(p.Surface)','Shadow(']),
    'Native screenshot-oriented tests declared': ('design_test.go',['TestOriginalFivePageVisualHierarchy','TestMyGoRenderDrawsTitlebarCanvasAndDock']),
}
miss = 0
for label, (f, needles) in checks.items():
    absent = [n for n in needles if n not in files[f]]
    print(('FAIL' if absent else 'PASS') + ' ' + label + (' — absent '+', '.join(absent) if absent else ''))
    miss += bool(absent)
# Compare structured Go constants rather than gofmt-dependent whitespace.
for name, value in {
    'TitlebarHeight':56, 'StatusbarHeight':26,
    'MainWindowWidth':1280,'MainWindowHeight':820,
    'TrayWidth':328,'TrayHeight':520,
    'FloatingDockIcon':32, 'AccountMinWidth':280,
    'SettingsSidebarWidth':220, 'OriginalNarrowBreakpoint':1240,
    'NarrowDockIcon':28, 'NarrowDockGap':6,
}.items():
    if not re.search(r"\b"+name+r"\s*=\s*"+str(value)+r"\b", geom):
        print('FAIL original geometry token '+name+'='+str(value))
        miss += 1
manifest_path = root / 'reference/visual/baseline-sha256.json'
try:
    manifest=json.loads(manifest_path.read_text())
    for name,digest in manifest.items():
        path=root/'reference/visual'/name
        if not path.is_file() or hashlib.sha256(path.read_bytes()).hexdigest()!=digest:
            print('FAIL immutable legacy reference: '+name)
            miss += 1
except (OSError,ValueError) as ex:
    print('FAIL missing baseline manifest:',ex)
    miss += 1
if not (d/'assets/lam-orbit-icon.png').is_file():
    print('FAIL original logo asset missing')
    miss += 1
try:
    import struct
    dims=json.loads((root/'reference/visual/baseline-dimensions.json').read_text())
    for name, expected in dims.items():
        png=(root/'reference/visual'/name).read_bytes()
        actual=struct.unpack('>II',png[16:24])
        if actual != (expected['widthPixels'],expected['heightPixels']):
            print('FAIL original screenshot dimensions changed: '+name)
            miss += 1
except (OSError,ValueError,KeyError) as ex:
    print('FAIL screenshot geometry manifest:',ex)
    miss += 1

print(f'Visual structural checks: {len(checks)-sum(0 for _ in [])} checks, {miss} failures. This is NOT a rendered native UI comparison.')
sys.exit(1 if miss else 0)
