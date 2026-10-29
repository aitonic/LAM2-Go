#!/usr/bin/env python3
"""Generate a per-declaration CSS -> native UI review ledger.

This is a REVIEW INVENTORY, not an engine that applies CSS to MyGo. A
registered original TSX occurrence is only a source-search clue, not proof
that a selector was rendered. No row becomes native PASS without verified
computed style and an actual pixel screenshot evidence record.
"""
import argparse
import csv
import hashlib
import json
import re
from collections import Counter, defaultdict
from pathlib import Path

ROOT = Path(__file__).resolve().parents[1]
SOURCE = ROOT / 'reference/visual'
STYLES = SOURCE / 'original-styles-css.txt'
DECLARATIONS = SOURCE / 'legacy-style-declarations.json'
CONTRACTS = SOURCE / 'legacy-style-contracts.json'
DEFAULT_OUTPUT = ROOT / 'reports/visual-trace'

MODULES = {
    'Global / tokens':'internal/desktop/design.go; internal/desktop/background.go; internal/visualspec/geometry.go',
    'Chrome / Dock':'internal/desktop/view.go; internal/desktop/brand.go',
    'Overview / account':'internal/desktop/overview.go; internal/desktop/design.go',
    'Usage / analytics':'internal/desktop/view.go; internal/desktop/activity.go; internal/desktop/design.go',
    'Provider / API':'internal/desktop/view.go; internal/desktop/forms.go',
    'Tray / menu':'internal/desktop/run.go; internal/desktop/tray_tokens.go',
    'Settings':'internal/desktop/settings.go',
    'Sessions / handoff':'internal/desktop/view.go',
    'Dialogs / forms':'internal/desktop/forms.go',
    'Other':'internal/desktop/view.go; internal/desktop/overview.go; internal/desktop/forms.go (find exact owner)',
}
TYPES = {
    'layout': set('display flex flex-direction flex-wrap flex-grow flex-shrink flex-basis grid-template-columns grid-template-rows grid-column grid-row grid-area align-items align-content justify-items justify-content place-items position top bottom left right inset z-index overflow overflow-x overflow-y object-fit box-sizing'.split()),
    'dimensions': set('width min-width max-width height min-height max-height aspect-ratio'.split()),
    'spacing': set('padding padding-top padding-right padding-bottom padding-left padding-inline padding-block margin margin-top margin-bottom margin-left margin-right gap row-gap column-gap'.split()),
    'type': set('font font-family font-size font-weight font-style font-feature-settings font-variant font-variant-numeric line-height letter-spacing word-break overflow-wrap white-space text-overflow text-transform text-decoration text-align'.split()),
    'effects': set('box-shadow filter backdrop-filter -webkit-backdrop-filter transition animation transform opacity background-clip -webkit-background-clip -webkit-text-fill-color'.split()),
    'surface': set('background background-image background-color color border border-color border-width border-style border-radius border-top border-bottom border-left border-right outline outline-color'.split()),
    'interaction': set('cursor pointer-events user-select appearance -webkit-appearance'.split()),
}
REGEX_CLASS = re.compile(r'(?<![\\\w])\.([A-Za-z_][A-Za-z\d_-]*)')


def classify(prop):
    if prop.startswith('--'): return 'theme/variable'
    for cat, values in TYPES.items():
        if prop in values or (prop.startswith('border-') and cat == 'surface'):
            return cat
    return 'misc'


def original_components(legacy_src):
    """Return searchable CODE references, not CSS matches or claims of reachability."""
    locations = defaultdict(list)
    if not legacy_src or not legacy_src.exists():
        cached=SOURCE/'original-component-token-index.json'
        if cached.exists():
            return defaultdict(list,json.loads(cached.read_text()))
        return locations
    for f in sorted(legacy_src.rglob('*')):
        if f.suffix not in ('.ts','.tsx','.js','.jsx') or '.test.' in f.name:
            continue
        if f.name.endswith('.d.ts'):
            continue
        lines = f.read_text(errors='replace').splitlines()
        for n,line in enumerate(lines,1):
            if not line.strip() or len(line) > 1000:
                continue
            for token in set(re.findall(r'\b[A-Za-z_][A-Za-z0-9_-]{2,}\b', line)):
                # An exact lexical token is only a candidate for DOM usage.
                if len(locations[token]) < 3:
                    try: rel=str(f.relative_to(legacy_src))
                    except ValueError: rel=str(f)
                    locations[token].append(f'{rel}:{n}')
    (SOURCE/'original-component-token-index.json').write_text(json.dumps(locations,ensure_ascii=False,sort_keys=True)+'\n')
    return locations


def acceptance(area, prop, selector, context):
    checks = ['MyGo Inspector 对应元素 Computed 样式/盒模型与原值一致', '在相同状态、窗口尺寸、DPI、语言和数据下的真实截图差异已核查']
    if prop in TYPES['interaction'] or ':hover' in selector or ':focus' in selector or ':disabled' in selector or ':active' in selector:
        checks.append('原生焦点/悬停/禁用/按下交互自动化测试实际通过')
    if '@media' in context or '@container' in context:
        checks.append('条件内外宽度两侧截图与布局测量均通过')
    if prop in TYPES['effects'] or '@keyframes' in context or prop == 'animation':
        checks.append('动态状态下帧序列/阴影/滤镜效果经实机确认')
    if area == 'Tray / menu':
        checks.append('浅色与深色、展开与滚动两种 Tray 状态核验')
    return '; '.join(checks)


def priority(area, kind, ctx, state):
    if state == 'KNOWN_NON_EQUIVALENCE': return 'P0'
    if 'media' in ctx or 'container' in ctx: return 'P0' if kind in ('layout','dimensions') else 'P1'
    if area in ('Global / tokens','Chrome / Dock','Dialogs / forms'):return 'P0' if kind in ('layout','dimensions','spacing','surface','type') else 'P1'
    if area in ('Overview / account','Usage / analytics','Tray / menu','Settings','Provider / API','Sessions / handoff'):
        return 'P0' if kind in ('layout','dimensions') else 'P1'
    return 'P2'


def report(legacy_src, output):
    raw=json.loads(DECLARATIONS.read_text())
    declarations=raw['declarations']
    contracts=json.loads(CONTRACTS.read_text())
    sha=hashlib.sha256(STYLES.read_bytes()).hexdigest()
    assert sha == raw['metadata']['sha256'], 'original CSS baseline hash changed'
    assert len(declarations) == 3504, 'baseline declaration count changed'
    assert len(set(x['id'] for x in declarations)) == len(declarations), 'ID collision'
    assert len(contracts) == raw['metadata']['contracts'], 'contract count mismatch'
    lookup={(x['context'],x['selector'],x['property'],x['expected']):x for x in contracts}
    index=original_components(legacy_src)
    rows=[]; units={}
    for r in declarations:
        cat=classify(r['property'])
        context,sel=r['context'],r['selector']
        key=(context,sel,r['property'],r['legacy_value'])
        contract=lookup.get(key)
        classes=REGEX_CLASS.findall(sel)
        refs=[]
        for cl in classes:
            for location in index.get(cl,[]):
                if location not in refs:refs.append(location)
        refs=refs[:4]
        lexical='CODE_TOKEN_FOUND_NOT_RUNTIME_PROVEN' if refs else ('GLOBAL_OR_ELEMENT' if not classes else 'SOURCE_CLASS_NOT_LOCATED')
        status=r['comparison']
        native='NOT_RUN'
        action={
            'SOURCE_MATCH_RENDER_UNTESTED':'在真实 MyGo 控件上验证此参数及其继承/状态覆盖；补原生断言',
            'STRUCTURE_MAPPED_RENDER_UNTESTED':'在断点两侧使用 Inspector 与实机截图验证尺寸/排列',
            'KNOWN_NON_EQUIVALENCE':'选择 MyGo 原生等效渲染实现并进行并排截图验证；当前视为缺口',
            'RESPONSIVE_NOT_VERIFIED':'需要在原版相同断点/状态下捕获计算样式，验证新框架条件规则',
            'EFFECT_NOT_VERIFIED':'需要 Hover/Focus/动画/阴影等状态对照；仅静态截图不充分',
            'NOT_VERIFIED':'逐控件落地或证明原版该规则不再生效；缺源码映射不能标为已对齐',
        }.get(status,'核验原生渲染')
        if lexical=='SOURCE_CLASS_NOT_LOCATED':
            action+='；原 TSX 中暂未找到类名使用，必须手动查动态/历史代码后才可裁定不适用'
        pr=priority(r['area'],cat,context,status)
        entry={
           'ID':r['id'], '优先级':pr,'页面/模块':r['area'],'CSS 行':r['line'],
           '规则起始行':r['rule_line'],'条件/主题':context,'CSS 选择器':sel,'属性类别':cat,
           'CSS 属性':r['property'],'原版精确值':r['legacy_value'],
           '原版组件源码候选':'; '.join(refs) if refs else '(无代码词法引用或仅全局/HTML 规则)',
           '原版组件检索状态':lexical,'新版候选模块':contract['file'] if contract else MODULES[r['area']],
           '新版静态值证据':r.get('new_code_evidence',''), '当前静态核验':status,
           '真实原生渲染验收':native,'逐项 Review 结论':action,
           '完成的必要证据':acceptance(r['area'],r['property'],sel,context),
        }
        rows.append(entry)
        uid=(context,r['rule_line'],sel)
        if uid not in units:
            units[uid]={'条件/主题':context,'CSS规则行':r['rule_line'],'选择器':sel,'页面/模块':r['area'],'属性条数':0,'已映射条数':0,'已知不等价条数':0,'响应/效果未验收条数':0,'未建立源码映射条数':0,'原版组件引用':entry['原版组件源码候选'],'新版候选模块':MODULES[r['area']]}
        u=units[uid];u['属性条数']+=1
        if status=='SOURCE_MATCH_RENDER_UNTESTED': u['已映射条数']+=1
        if status=='KNOWN_NON_EQUIVALENCE':u['已知不等价条数']+=1
        if status in ('RESPONSIVE_NOT_VERIFIED','EFFECT_NOT_VERIFIED'):u['响应/效果未验收条数']+=1
        if status in ('NOT_VERIFIED','KNOWN_NON_EQUIVALENCE'):u['未建立源码映射条数']+=1
    rules=[]
    for i,(key,u) in enumerate(units.items(),1):
        u['Review ID']=f'RULE-{i:04d}'
        u['最终状态']='NATIVE_UNVERIFIED'
        u['需要操作']='对本规则全部声明逐项建立 Go 控件与原生状态证据；禁止仅以文件/常量匹配放行'
        rules.append(u)
    output.mkdir(parents=True,exist_ok=True)
    def write_csv(path,values):
        with path.open('w',encoding='utf-8-sig',newline='') as f:
            w=csv.DictWriter(f,fieldnames=list(values[0]));w.writeheader();w.writerows(values)
    write_csv(output/'all-3504-css-review.csv',rows)
    write_csv(output/'all-css-rules-review.csv',rules)
    for source,filename in ((ROOT/'docs/FEATURE-TEST-TRACEABILITY.csv','feature-239-review.csv'),):
        # Preserve the independent functional acceptance ledger in this bundle.
        with source.open('r',encoding='utf-8-sig',newline='') as f:
            items=list(csv.DictReader(f))
        if items:write_csv(output/filename,items)
    screen_names=['OverView','Usage','Settings','Bar','AddAPI','AddAcount']
    states=['默认浅色','深色','Hovered / Focused','错误 / 空数据 / Loading','弹窗 / 菜单展开','窄窗口与断点','长文本与滚动','键盘与中文输入','实际 Retina DPI/缩放','真实截图与逐元素 Inspector']
    scenarios=[]
    for name in screen_names:
        for n,state in enumerate(states,1):
            scenarios.append({'ID':f'VIS-{name.upper()}-{n:02d}','原版屏幕':name+'.png','状态':state,'真实 MyGo 截图':'缺失','实际对比':'NOT_RUN','验收标准':'匹配真实页面数据、窗口内容尺寸与 UI 状态；逐项确认布局与视觉差异','说明':'原版仅提供静态截图；动态/暗色需补原版捕获' if state!='默认浅色' else '待确认原始截图主题/数据'})
    write_csv(output/'visual-state-60-review.csv',scenarios)
    stats={'css_declarations':len(rows),'css_rule_groups':len(rules),'legacy_css_sha256':sha,'per_decl_state':dict(Counter(x['当前静态核验'] for x in rows)),'priorities':dict(Counter(x['优先级'] for x in rows)),'original_component_reference':dict(Counter(x['原版组件检索状态'] for x in rows)),'per_area':dict(Counter(x['页面/模块'] for x in rows)),'native_pass_count':0,'enumerated_for_review':len(rows),'actual_native_verified':False,'visual_scenarios':len(scenarios),'features':len(items)}
    (output/'review-status.json').write_text(json.dumps(stats,indent=2,ensure_ascii=False)+'\n')
    print(json.dumps(stats,indent=2,ensure_ascii=False))

if __name__=='__main__':
    ap=argparse.ArgumentParser();ap.add_argument('--legacy-src',type=Path);ap.add_argument('--out',type=Path,default=DEFAULT_OUTPUT)
    args=ap.parse_args();report(args.legacy_src,args.out)
