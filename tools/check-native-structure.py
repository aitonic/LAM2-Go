#!/usr/bin/env python3
"""Check code-level visual invariants before any real MyGo screenshot comparison.

These are source checks, NEVER native/pixel parity. Every checked invariant comes
from the exact original LAM 0.4.4 stylesheet + component structure.
"""
from pathlib import Path
import re
import sys
ROOT = Path(__file__).resolve().parents[1]
view=(ROOT/'internal/desktop/view.go').read_text()
design=(ROOT/'internal/desktop/design.go').read_text()
geometry=(ROOT/'internal/visualspec/geometry.go').read_text()
activity=(ROOT/'internal/desktop/activity.go').read_text()
legacy=(ROOT/'reference/visual/original-styles-css.txt').read_text()
checks=[]
def assert_source(name, ok): checks.append((name,bool(ok)))
metric_match=re.search(r'func metricCard\(.*?\n}\n',design,re.S)
assert_source('original .metricCard has no fixed CSS height',bool(re.search(r'\.metricCard\s*\{[^}]*?padding:\s*16px 18px;',legacy,re.S)) and bool(metric_match) and not re.search(r'\.Height\(',metric_match.group(0) if metric_match else ''))
assert_source('original Sessions 1280px minimum',bool(re.search(r'\.sessionsTable\s*\{[^}]*?min-width:\s*1280px',legacy,re.S)) and bool(re.search(r'SessionsTableMinWidth\s*=\s*1280',geometry)))
assert_source('Sessions exact six columns',all(f'Title: "{col}"' in view for col in ('Session','Size','Last active','cwd','Provider','Actions')))
assert_source('Sessions six column ratios',bool(re.search(r'pct := \[6\]int\{22, 7, 13, 16, 12, 30\}',geometry)))
assert_source('Sessions row actions in native Table',all(k in view for k in ('Relay To...','>_ Terminal','u.copyResume(s.ProfileID, s.ID, s.CWD)','u.core.Sessions.Read(ctx, s.ProfileID, s.ID)')))
assert_source('Sessions table based on native ui.Table', 'ui.Table(c, &u.sessionTable, cols, len(u.sessions.Items)' in view)
assert_source('Usage page fixed 18px padding', 'Padding(visualspec.UsagePagePadding)' in view)
assert_source('Usage page fixed 14px gap', 'Gap(visualspec.UsagePageGap)' in view)
assert_source('Usage header 22px title', 'visualspec.UsageHeaderTitleSize' in view)
assert_source('Usage header 34px icon', 'visualspec.UsageHeaderLogoSize' in view)
assert_source('Heatmap 14px cell 3px gutter', 'visualspec.HeatmapCellSize' in activity and 'visualspec.HeatmapCellGap' in activity)
assert_source('Heatmap both theme levels',all(v in activity for v in ('#ebedf0','#fbcfe8','#f472b6','#ec4899','#db2777','#1e293b','#4a153b','#7c1a60','#ae1e88')))
assert_source('1240px titlebar wrap and padding', 'header.Wrap().Padding(visualspec.NarrowTitlebarPadY, visualspec.NarrowTitlebarPadX)' in view)
errors=[n for n,ok in checks if not ok]
for name,ok in checks:print(('PASS ' if ok else 'FAIL ')+name)
print(f'Native code structure checks: {len(checks)}; failures: {len(errors)}. DOES NOT certify actual rendering.')
sys.exit(bool(errors))
