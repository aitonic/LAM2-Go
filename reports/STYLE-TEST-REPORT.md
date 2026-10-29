# Style repair — executed tests (2026-10-29)

- Prior LAM2 delivered a mostly unstyled native UI. The correction is in `internal/desktop/design.go`, `overview.go`, `view.go`, `activity.go`, `settings.go`, `forms.go`, and `run.go`.
- Original screenshot evidence copied into `reference/visual/` (6 PNG files).
- `./tools/check-visual-style.py`: **PASS 15 / 15 source assertions**. Source-only, not pixel/render acceptance.
- `./tools/check-source.sh`: **PASS**. No legacy language/HTML/CSS runtime added.
- `./tools/test-offline.sh json`: **PASS**; 186 top-level Go tests and 207 pass records including subtests, no failures. Test set excludes native Go modules.
- `./tools/test-offline.sh vet`: **PASS**.
- `./tools/test-offline.sh race`: **INCOMPLETE** — run timed out during compilation/execution after 4 packages had passed. Not counted as success.
- Real `ui.NewTester` visual tests: **TEST FILES ADDED, NOT RUN** (requires Go 1.27.3+ / MyGo pinned dependencies).
- Real `ui.Render` / screenshot differences: **NOT RUN**.
- `tools/check-native-visual-tests.py` negative (missing native test) input: **PASS** — returned failure as expected. This tests the *gate*, not the view.
- Full MyGo build / native tests / macOS screenshots / DMG: **UNVERIFIED**. Current environment has Go 1.23.2; network cannot download Go 1.27.3.

**Do not release this as fully style-parity verified.**
