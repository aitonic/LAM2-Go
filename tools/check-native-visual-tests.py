#!/usr/bin/env python3
"""Require a green, non-skipped Go execution for each native style test.

The source-only visual audit is deliberately insufficient for this gate.
This must run after the real MyGo tests on a supported Go toolchain.
"""
import json, sys
from pathlib import Path
if len(sys.argv)!=2:
    sys.exit('usage: check-native-visual-tests.py reports/native-tests.jsonl')
required={
'TestNativeStylesLightAndDarkHaveIndependentTokens',
'TestOriginalFivePageVisualHierarchy',
'TestOverviewAccountCardSelectsExistingOperations',
'TestMyGoRenderDrawsTitlebarCanvasAndDock',
'TestOriginalOverviewAgentTabsAndSourceBar',
'TestTrayVisualCardsAcrossSystemTheme',
'TestUsageNativeActivityHeatmapHasHorizontalLegend',
'TestOriginalAccountModalLayoutAndTabs',
'TestOriginalProviderEditorLayout',
'TestSettingsOriginalInformationArchitecture',
'TestUsageTabsOriginalOrderAndSelection',
}
passed=set(); failed=set(); package_pass=False
path=Path(sys.argv[1])
try:
    with path.open() as f:
        for line in f:
            try: event=json.loads(line)
            except json.JSONDecodeError: continue
            if not str(event.get('Package','')).endswith('/internal/desktop'):
                continue
            act=event.get('Action'); name=event.get('Test','')
            if not name and act=='pass':package_pass=True
            if name in required:
                if act=='pass':passed.add(name)
                if act in ('fail','skip'):failed.add(name)
except OSError as e: sys.exit(str(e))
missing=required-passed
if not package_pass or missing or failed:
    print(f'FAIL visual native gate: package_pass={package_pass}; missing={sorted(missing)}; failed/skipped={sorted(failed)}', file=sys.stderr)
    sys.exit(1)
print(f'PASS native visual gate: {len(required)} named tests + desktop package passed')
