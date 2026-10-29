import re,sqlite3,json,pathlib
root=pathlib.Path(__file__).resolve().parent.parent
s=(root/'internal/integrations/mygosqlite/sqlite.go').read_text()
l=(root/'internal/integrations/mygosqlite/legacy.go').read_text()
c=sqlite3.connect(':memory:')
for q in re.findall(r'\{SQL: `(CREATE [^`]+)`\}',s):c.execute(q)
qs=dict(re.findall(r'const (publish\w+SQL) = `([^`]+)`',l))
assert len(qs)==3
stage='INSERT INTO legacy_event_stage VALUES('+','.join(['?']*15)+')'
payload=json.dumps({'id':'old-id','threadKey':'thread-a'})
args=['batch-a','old-id','p','s','/work','m','2026-10-01T00:00:00.000000000Z',100,40,20,5,None,'legacy-import',payload,'raw:3:120']
c.execute(stage,args)
c.execute(qs['publishLegacyEventsSQL'],['batch-a'])
c.execute(qs['publishLegacyOccurrencesSQL'],['legacy:batch-a','batch-a'])
c.execute(qs['publishLegacyAliasesSQL'],['batch-a'])
assert c.execute('select count(*),sum(input_tokens+output_tokens) from usage_events').fetchone()==(1,120)
insert=re.search(r'SQL: `(INSERT INTO usage_events[^`]+)`',s).group(1)
a=['legacy-record:raw:3:120','new-id','p','s','/work','m','2026-10-01T00:00:00.000000000Z',100,40,20,5,None,'new-price',payload]
c.execute(insert,a)
occ=re.search(r'SQL: "(INSERT OR IGNORE INTO source_occurrences\(source_key,event_id\) VALUES[^"\n]+)"',s).group(1)
c.execute(occ,['source-a','legacy-record:raw:3:120','new-id'])
assert c.execute('select count(*) from usage_events').fetchone()[0]==1
c.execute('DELETE FROM source_occurrences WHERE source_key=?',['source-a'])
c.execute('DELETE FROM usage_events WHERE NOT EXISTS(SELECT 1 FROM source_occurrences WHERE event_id=usage_events.id)')
assert c.execute('select count(*) from usage_events').fetchone()[0]==1
c.execute('DELETE FROM source_occurrences WHERE source_key=?',['legacy:batch-a'])
c.execute('DELETE FROM usage_events WHERE NOT EXISTS(SELECT 1 FROM source_occurrences WHERE event_id=usage_events.id)')
assert c.execute('select count(*) from usage_events').fetchone()[0]==0
print(json.dumps({'result':'PASS','engine':'Python sqlite3 '+sqlite3.sqlite_version,'source':'literal SQL extracted from production Go files','checks':['schema DDL','legacy publication','alias deduplication across JSONL reindex','reset retains imported occurrence','rollback removes import-only event'],'limitation':'Does not execute MyGo plugin, Go adapter, WAL reader or native integration tests.'},indent=2))
