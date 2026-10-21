"""Shift every timestamp in a fixture database so the earliest one is BASE,
merge the search index, and compact the file."""
import re, sqlite3, sys
from datetime import datetime, timedelta, timezone

BASE = datetime(2025, 1, 2, 15, 4, 5, tzinfo=timezone.utc)
RFC = re.compile(r'^(\d{4}-\d\d-\d\dT\d\d:\d\d:\d\d)(\.\d+)?Z$')
PLAIN = re.compile(r'^\d{4}-\d\d-\d\d \d\d:\d\d:\d\d$')

def parse(v):
    m = RFC.match(v)
    if m:
        return datetime.strptime(m.group(1), '%Y-%m-%dT%H:%M:%S').replace(tzinfo=timezone.utc), m.group(2) or '', 'rfc'
    if PLAIN.match(v):
        return datetime.strptime(v, '%Y-%m-%d %H:%M:%S').replace(tzinfo=timezone.utc), '', 'plain'
    return None

def fmt(t, frac, kind):
    return t.strftime('%Y-%m-%dT%H:%M:%S') + frac + 'Z' if kind == 'rfc' else t.strftime('%Y-%m-%d %H:%M:%S')

c = sqlite3.connect(sys.argv[1])
c.execute('PRAGMA wal_checkpoint(TRUNCATE)')
tables = [r[0] for r in c.execute("select name from sqlite_master where type='table' and sql not like 'CREATE VIRTUAL%' and name not like 'run_search_%' and name != 'sqlite_sequence'")]
cells = []
for t in tables:
    cols = [r[1] for r in c.execute(f"pragma table_info('{t}')")]
    for col in cols:
        for rowid, v in c.execute(f"select rowid, \"{col}\" from '{t}'"):
            if isinstance(v, str) and (p := parse(v)):
                cells.append((t, col, rowid, p))
earliest = min(p[0] for *_, p in cells)
delta = BASE - earliest
for t, col, rowid, (when, frac, kind) in cells:
    c.execute(f"update '{t}' set \"{col}\"=? where rowid=?", (fmt(when + delta, frac, kind), rowid))
print(f"{sys.argv[1]}: shifted {len(cells)} timestamps by {delta}")
if c.execute("select 1 from sqlite_master where name='run_search'").fetchone():
    c.execute("insert into run_search(run_search) values('optimize')")
c.commit()
c.execute('VACUUM')
c.close()
