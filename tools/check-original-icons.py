#!/usr/bin/env python3
"""Verify embedded MyGo native SVG icons retain exact LAM 0.4.4 paths.

This is an asset fidelity check only; it cannot certify native rasterization.
"""
from pathlib import Path
import hashlib, re, sys, xml.etree.ElementTree as ET
root=Path(__file__).resolve().parents[1]
src=(root/'reference/visual/original-icons-tsx.txt').read_text()
icons={'overview':'Overview','usage':'Usage','sessions':'Sessions','providers':'Providers','settings':'Settings','accounts':'Users','activity':'Activity','quota':'Clock','device':'Device','sun':'Sun','moon':'Moon','refresh':'Refresh'}
errors=[]
for name, tag in icons.items():
    match=re.search(r'export function Icon'+tag+r'\(props: IconProps\) \{\s*return \(\s*<Svg \{\.\.\.props\}>(.*?)</Svg>\s*\);\s*\}',src,re.S)
    if match is None:
        errors.append(f'{name}: no source icon')
        continue
    path=root/f'internal/desktop/assets/icons/{name}.svg'
    if not path.exists():
        errors.append(f'{name}: no svg asset')
        continue
    try:
        original=ET.fromstring('<svg>'+match.group(1)+'</svg>')
        native=ET.parse(path).getroot()
        old=[(x.tag,sorted(x.attrib.items())) for x in original]
        new=[(x.tag.split('}')[-1],sorted(x.attrib.items())) for x in native]
        if old != new:
            errors.append(f'{name}: SVG child shapes differ from old source')
    except ET.ParseError as e:
        errors.append(f'{name}: malformed SVG: {e}')
tray_src=root/'reference/visual/original-tray-menu-template.png'
tray_new=root/'internal/desktop/assets/tray-menu-template.png'
if not tray_src.exists() or not tray_new.exists() or hashlib.sha256(tray_src.read_bytes()).digest()!=hashlib.sha256(tray_new.read_bytes()).digest():
    errors.append('tray bitmap differs from original Tauri resource')
app_src=root/'reference/visual/original-app-icon.png'
app_new=root/'resources/icon.png'
if not app_src.exists() or not app_new.exists() or hashlib.sha256(app_src.read_bytes()).digest()!=hashlib.sha256(app_new.read_bytes()).digest():
    errors.append('application bundle icon differs from original LAM icon')
css=(root/'reference/visual/original-styles-css.txt').read_text()
for original_stop in ('radial-gradient(circle at center, #ffffff 0%, #eef0f4 100%)','radial-gradient(circle at center, #161a24 0%, #0b0d12 100%)'):
    if original_stop not in css:
        errors.append('CSS radial baseline unexpectedly changed')
for name, colors in (('light',('#ffffff','#eef0f4')),('dark',('#161a24','#0b0d12'))):
    text=(root/f'internal/desktop/assets/bg/{name}.svg').read_text()
    if not all(c in text for c in colors) or '<radialGradient' not in text:
        errors.append(f'{name}: missing original radial gradient stops')
print(f'Original SVG paths: {len(icons)} validated; original app/tray templates and background stops: '+('PASS' if not errors else 'FAIL'))
for e in errors: print('FAIL '+e,file=sys.stderr)
sys.exit(1 if errors else 0)
