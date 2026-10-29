#!/usr/bin/env python3
"""Validate the completeness of the original CSS / feature / state review.

An inventory pass is NOT a visual fidelity pass. With --release, every active
CSS declaration additionally needs a real native computed-style and image
reference; those must be validated by check-native-screenshots.sh as well.
"""
from pathlib import Path
import csv
import hashlib
import json
import sys

ROOT=Path(__file__).resolve().parents[1]
REFERENCE=ROOT/'reference/visual'
DIR=ROOT/'reports/visual-trace'

def read_csv(name):
    path=DIR/name
    if not path.exists():raise RuntimeError(f'missing ledger: {name}')
    with path.open(encoding='utf-8-sig',newline='') as f:
        return list(csv.DictReader(f))

def check():
    errors=[]
    origin=json.loads((REFERENCE/'legacy-style-declarations.json').read_text())
    ids=[r['id'] for r in origin['declarations']]
    sha=hashlib.sha256((REFERENCE/'original-styles-css.txt').read_bytes()).hexdigest()
    if sha!=origin['metadata']['sha256']:errors.append('original CSS source SHA changed')
    css=read_csv('all-3504-css-review.csv')
    rules=read_csv('all-css-rules-review.csv')
    features=read_csv('feature-239-review.csv')
    states=read_csv('visual-state-60-review.csv')
    if len(css)!=len(ids) or [r['ID'] for r in css]!=ids:
        errors.append('CSS ID / row order / count mismatch')
    if len(rules)!=819 or sum(int(r['属性条数']) for r in rules)!=len(ids):
        errors.append('CSS rule groups are missing or duplicate')
    if len(features)!=239 or len(set(r['feature_id'] for r in features))!=239:
        errors.append('239-feature matrix incomplete')
    if len(states)!=60 or len(set(r['ID'] for r in states))!=60:
        errors.append('native visual-state cases incomplete')
    for row in css:
        if row['真实原生渲染验收'] not in ('NOT_RUN','NATIVE_VERIFIED'):
            errors.append('invalid native result status: '+row['ID'])
        if row['真实原生渲染验收']=='NATIVE_VERIFIED':
            # Prevent marking a CSS row green by merely editing a CSV cell.
            if not (ROOT/'reports/native-screenshots').exists() or not (ROOT/'reports/native-computed').exists():
                errors.append('native verification claimed with no screenshot AND computed-style evidence: '+row['ID'])
    if '--release' in sys.argv:
        if any(r['真实原生渲染验收']!='NATIVE_VERIFIED' for r in css):
            errors.append('native release blocked: unresolved CSS declarations remain')
        if any(r['实际对比']!='PASS' for r in states):
            errors.append('native release blocked: visual states not all validated')
        if any(r['test_result'] not in ('TEST_PASS','SKIPPED_RETIRED') for r in features):
            errors.append('release blocked: functional parity tests incomplete')
    return errors,len(css),len(rules),len(features),len(states)

if __name__=='__main__':
    try:errors,ncss,nrules,nfeat,nstates=check()
    except Exception as exc:errors=[str(exc)];ncss=nrules=nfeat=nstates=0
    print(f'Trace ledger: CSS={ncss}, rules={nrules}, features={nfeat}, visual states={nstates}; errors={len(errors)}')
    for err in errors[:20]: print('FAIL:',err)
    if len(errors)>20:print(f'... {len(errors)-20} additional errors')
    sys.exit(bool(errors))
