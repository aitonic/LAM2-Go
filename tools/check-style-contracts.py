#!/usr/bin/env python3
"""Source-level CSS-to-Go mapping checker and strict native visual parity gate.

The default mode validates each *declared* mapping against the original CSS
and Go source. --require-all refuses release until every declaration is linked
and verified against a native computed-style/screenshot artifact. No static
contract can make that claim by itself.
"""
from pathlib import Path
import hashlib
import json
import re
import sys
from collections import Counter

ROOT=Path(__file__).resolve().parents[1]
BASE=ROOT/'reference/visual'
raw=json.loads((BASE/'legacy-style-declarations.json').read_text())
contracts=json.loads((BASE/'legacy-style-contracts.json').read_text())
rows=raw['declarations']


def norm(value):
    return re.sub(r'\s+', ' ', value.strip())


def check():
    errors=[]
    if len(rows)!=raw['metadata']['declarations']:
        errors.append('declaration count differs from original extraction')
    original=BASE/'original-styles-css.txt'
    if not original.is_file():
        errors.append('original stylesheet fixture missing')
    elif hashlib.sha256(original.read_bytes()).hexdigest()!=raw['metadata']['sha256']:
        errors.append('original stylesheet SHA-256 differs: baseline may have been modified')
    ids={v['id'] for v in rows}
    if len(ids)!=len(rows):
        errors.append('declaration ID collision')
    if len(contracts)!=raw['metadata']['contracts']:
        errors.append('declared contract count mismatch')
    indexed={(v['context'], v['selector'], v['property'], v['legacy_value']) for v in rows}
    seen=set()
    for c in contracts:
        identity=(c['context'], c['selector'], c['property'], c['expected'])
        if identity not in indexed:
            errors.append('legacy CSS declaration missing/mismatched: '+c['label'])
        if identity in seen:
            errors.append('duplicate evidence contract: '+c['label'])
        seen.add(identity)
        source=ROOT/c['file']
        if not source.is_file():
            errors.append('Go source file missing: '+c['file'])
        elif norm(c['go_evidence']) not in norm(source.read_text()):
            errors.append('Go code evidence missing/mismatched: '+c['label'])
        if 'use_evidence' in c and norm(c['use_evidence']) not in norm(source.read_text() if source.is_file() else ''):
            errors.append('Go color definition is not connected to UI: '+c['label'])
        if 'constant_evidence' in c:
            constant_file=ROOT/c.get('constant_file','')
            if not constant_file.is_file() or norm(c['constant_evidence']) not in norm(constant_file.read_text()):
                errors.append('Go constant definition missing/mismatched: '+c['label'])
    return errors


errors=check()
for e in errors:
    print('FAIL:',e)
counts=Counter(v['comparison'] for v in rows)
print('Original CSS:',len(rows),'properties; source mappings:',len(contracts),
      '; actual native render matches:',counts.get('NATIVE_VERIFIED',0),'; source check errors:',len(errors))
if '--require-all' in sys.argv:
    # Explicit strict gate: all 3504 declarations require native visual proof.
    # Merely moving a declaration into a Go constant is NOT sufficient.
    unverified=[v for v in rows if v['comparison']!='NATIVE_VERIFIED']
    if unverified:
        errors.append(f'{len(unverified)} of {len(rows)} CSS declarations have no native computed-style AND screenshot equivalence evidence')
        print('RELEASE BLOCKED:',errors[-1])
    else:
        print('Source mapping and native assertions are complete; run real screenshot gate next.')
else:
    print('Source-value checks do not certify computed style, interaction, or pixel parity.')
sys.exit(bool(errors))
