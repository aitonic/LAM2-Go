# LAM 0.4.4 -> Go + MyGo: Visual Fidelity Contract

**Status: source-level structure reviewed and corrected; native visual parity UNVERIFIED.**

Visual parity is an independently required acceptance condition. Passing business tests, source-token checks, or headless UI tests is insufficient to claim unchanged layout or style.

## Original source of truth

- `LAM.zip:apps/desktop/src/styles.css` -> `reference/visual/original-styles-css.txt`
- `LAM.zip:apps/desktop/src/components/icons.tsx` -> `reference/visual/original-icons-tsx.txt`
- `LAM.zip:apps/desktop/src-tauri/tauri.conf.json` -> geometry in `internal/visualspec/geometry.go`
- Six original screenshots in `reference/visual/*.png`, immutable SHA256 in `baseline-sha256.json`, dimensions in `baseline-dimensions.json`
- Original app icon and macOS tray template in `reference/visual/`, restored to `resources/icon.png` and `internal/desktop/assets/tray-menu-template.png`.

## Current source-alignment checkpoints

| Screen / element | Original baseline | MyGo source-level change | Result |
|---|---|---|---|
| Main window | default 1280x820; minimum 1100x700 | `run.go` and `visualspec` restored original values | Source aligned; native unverified |
| Global shell | titlebar 56 DIP; bottom status 26 DIP; original radial background | `view.go`, `background.go`, native SVG radial gradient | Source aligned; raster not compared |
| Bottom Dock | fixed floating above status, 32x32 icons, 10px labels; CSS blur(28px) | Native absolute overlay, exact original SVG icons, official `glass.Blur{Radius:28}` | Source aligned; saturation/filter/renderer unverified |
| Overview | four compact metric cards; Accounts tab/source; auto-fill min 280 DIP cards | no spurious metric top stripes, real metric icons, overlay account operations | Source aligned; card metadata/spacing not raster-verified |
| Usage | three tall KPI panels, pink 53-week heatmap, Insights / Calls / Threads / Diagnostics | native KPI layout/heatmap/tabs; source token colors | Source partly aligned; some data-cell content differs |
| Settings | 220 DIP flush left rail; APP SETTINGS / INTEGRATIONS / RESOURCES; preference rows | moved settings sections and native select labels | Source aligned; native control sizes not raster-verified |
| Tray | default 328x520 resizable; original menu-icon template; segmented account cards | original bitmap, 328x520, 30px brand icon, stats strip | Source aligned; row density/panel opacity not raster-verified |
| Add Account | header + auth mode + session JSON + CODEX_HOME preview + footer | original section ordering restored | Partial: wrapper checkbox disabled and import transactional equivalence pending |
| Add External API | header, Advanced options, two columns, footer buttons | native 780-DIP two-column modal | Partial: create-then-API-account flow differs |
| Sessions/Providers *main screens* | no screenshots in provided `LAM.zip` pictures | original CSS and UI components used as reference | Cannot claim screenshot fidelity without target captures |

## Checks and evidence levels

1. **Source geometry and assets**: `make visual-audit`, `go test ./internal/visualspec ./tools/visualdiff`. Original SVG paths, tray PNG, bundle icon, CSS radial stops, immutable screenshots and layout values must match. These checks are run in this handoff, with logs.
2. **Native UI behavior**: `ui.NewTester` tests in `internal/desktop/original_layout_test.go` plus `design_test.go` on the actual locked MyGo/Go toolchain. They are written but **NOT run** in the current environment.
3. **Actual rendering**: six native screenshots, each captured with the same sample data, locale, theme, window content rectangle, DPI/Retina scaling, UI state and app zoom as the corresponding original screenshot. Required output paths: `reports/native-screenshots/{OverView,Usage,Settings,Bar,AddAPI,AddAcount}.png`.
4. **Screenshot comparison**: `./tools/check-native-screenshots.sh` must pass with actual screenshots. It currently fails deliberately because the screenshots do not exist. A pixel-diff without identical data, font/OS chrome and viewport is invalid evidence; compare like-for-like and inspect remaining diff regions before release.
5. **Manual signoff**: macOS layout, scrolling, resize, focus, light/dark, IME, modal positioning, menus, hover, and native transparency/blur need real interactive acceptance, not just pixel counts.

### Baseline screenshot physical sizes

| Image | Pixels | Note |
|---|---|---|
| OverView.png | 3350 x 1942 | Live original app state, macOS chrome included |
| Usage.png | 3360 x 1932 | Live historical usage, data-dependent text |
| Settings.png | 3354 x 1940 | Native menu/options in captured state |
| Bar.png | 654 x 1410 | Tray manually resized taller than default |
| AddAPI.png | 1556 x 1216 | Original dialog capture |
| AddAcount.png | 1020 x 1506 | Original dialog capture |

The screenshot dimensions above are physical pixels of the source captures, **not** the original default window sizes in DIPs. They contain live data and sometimes OS window chrome. The release-gate comparator will not forgive missing actual screenshots.

### MyGo official capture path

The built-in `plugins/automation` supports `capture` and `idle` on a running Native UI window. This project has a dev-only Go file `automation_e2e.go` behind build tag `e2e` to use it. An E2E build runs with `MYGO_AUTOMATION` set to a private Unix socket; production builds omit it. The actual capture is a PNG from the running native window, not a generated mock. See https://mygo.egoist.dev/docs/plugins/automation .

## Known limits and false-certification guard

- React CSS/WebKit text shaping and MyGo native font/GPU rasterization can differ by pixels even when font, sizes and colors match. A stronger 'pixel identical' claim requires actual macOS captures.
- Original backdrop filter also uses `saturate(1.25)`. We restored its blur via MyGo `glass.Blur`, but saturation and material blending are not proven equivalent.
- Modal import/creation transactions and the disabled overwrite-wrapper option are outstanding **functional** differences, not just visual differences.
- Original usage cards show task duration and credit details not present in the current Go KPI composition. A source-styling check alone cannot accept exact content/layout parity here.
- Provider/Session main page reference screenshots are missing; require new original-app captures before claiming exact restoration.

**Decision: Do not mark visual parity PASS or ship the replacement until all native screenshots, functionality, and manual checks are verified on supported macOS.**
