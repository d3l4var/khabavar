#!/usr/bin/env python3
"""List SQL-bearing store methods that no PG test exercises.

Why: two production bugs (customer login 42P10, category replacement writing nothing)
were invisible to the unit suite because those paths only ever ran against an in-memory
store. Every method that executes SQL needs at least one run on the real schema.

Usage:  python3 scripts/audit-pg-coverage.py [--fail-under PCT]

Exit code is 1 when coverage is below --fail-under, so CI can gate on it once the
number is where we want it (currently advisory).
"""
import glob
import re
import sys

SQL_CALL = re.compile(r'pool\.(Exec|Query|QueryRow)|tx\.(Exec|Query|QueryRow)')
METHOD = re.compile(r'func \(s \*(\w+)\) (\w+)\((.*?)\) \((.*?)\) \{(.*?)\n\}', re.S)


def main() -> int:
    threshold = 0.0
    if '--fail-under' in sys.argv:
        threshold = float(sys.argv[sys.argv.index('--fail-under') + 1])

    rows = []
    for path in sorted(glob.glob('internal/modules/*/*_db.go')):
        module = path.split('/')[2]
        src = open(path, encoding='utf-8').read()
        methods = {m.group(2) for m in METHOD.finditer(src) if SQL_CALL.search(m.group(5))}
        pg = ''.join(open(p, encoding='utf-8').read()
                     for p in glob.glob(f'internal/modules/{module}/*pg_test.go'))
        for name in sorted(methods):
            rows.append((module, name, name in pg))

    covered = [r for r in rows if r[2]]
    uncovered = [r for r in rows if not r[2]]
    pct = 100.0 * len(covered) / len(rows) if rows else 100.0

    print(f"SQL-bearing store methods: {len(rows)}")
    print(f"covered by a PG test:      {len(covered)} ({pct:.1f}%)")
    print(f"uncovered:                 {len(uncovered)}")
    if uncovered:
        print("\nUNCOVERED (add a *_pg_test.go case before trusting these paths):")
        for module, name, _ in uncovered:
            print(f"  {module:12s} {name}")
    return 1 if pct < threshold else 0


if __name__ == '__main__':
    raise SystemExit(main())
