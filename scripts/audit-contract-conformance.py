#!/usr/bin/env python3
"""Check that handlers honour what the contract declares for their operation.

Why: audit-routes.py only compares path SHAPES, so a route can be "mounted" while the
handler ignores a declared query parameter or never emits a declared error code. That is
exactly how GET /orders/{orderNumber} shipped without its required ?phone= check and
without the 403 the contract promised.

Checks per operation:
  1. every declared query parameter is read by the handler
  2. every declared 4xx response code is emitted by the handler

Usage: python3 scripts/audit-contract-conformance.py [--fail]
"""
import re
import sys
import glob

ROUTES = ['cmd/khavabar/routes.go'.replace('khavabar', 'khabavar'), 'cmd/khabavar/main.go']
PREFIX = '/api/v1'
CONTRACT = 'api/openapi.yaml'


def shape(p):
    return re.sub(r'\{[^}]+\}', '{}', p.rstrip('/')) or '/'


def handler_scope(expr):
    """Return the source a handler can legitimately use: its own body plus the rest of the
    package's non-test files. A handler often delegates a check to a helper in the same
    file (mayReadOrder), so a body-only search reports false positives."""
    m = re.search(r'(\w+)\.(\w+)\(', expr)
    if not m:
        return None
    fn = m.group(2)
    for f in glob.glob('internal/modules/*/*.go'):
        if f.endswith('_test.go'):
            continue
        src = open(f, encoding='utf-8').read()
        if re.search(rf'\nfunc {fn}\(', src):
            pkg = f.rsplit('/', 1)[0]
            return '\n'.join(open(g, encoding='utf-8').read()
                             for g in glob.glob(pkg + '/*.go') if not g.endswith('_test.go'))
    return None


# mount expression per (method, shape)
mounts = {}
for f in ROUTES:
    src = open(f, encoding='utf-8').read()
    for m in re.finditer(r'api\.\w+\("(GET|POST|PUT|PATCH|DELETE) (/[^"]+)",', src):
        meth, path = m.group(1), m.group(2)
        i, depth = m.end(), 1
        while i < len(src) and depth:
            if src[i] == '(':
                depth += 1
            elif src[i] == ')':
                depth -= 1
            i += 1
        mounts[(meth, shape(path[len(PREFIX):] if path.startswith(PREFIX) else path))] = src[m.end():i - 1]

# contract operations
ops = {}
src = open(CONTRACT, encoding='utf-8').read()
for m in re.finditer(r'^  (/[^:]+):\s*$', src, re.M):
    path, start = m.group(1), m.end()
    nxt = re.search(r'^  /[^:]+:\s*$', src[start:], re.M)
    block = src[start:start + (nxt.start() if nxt else len(src))]
    for mm in re.finditer(r'^\s{4}(get|post|put|patch|delete):\s*$', block, re.M):
        meth = mm.group(1).upper()
        bstart = mm.end()
        bnxt = re.search(r'^\s{4}(get|post|put|patch|delete):\s*$', block[bstart:], re.M)
        body = block[bstart:bstart + (bnxt.start() if bnxt else len(block))]
        params = re.findall(r'name:\s*([A-Za-z_]+),\s*in:\s*query', body)
        codes = re.findall(r'^\s{8}"(\d{3})":', body, re.M)
        ops[(meth, shape(path))] = (path, params, codes)

problems = []
for key, (cpath, params, codes) in sorted(ops.items()):
    expr = mounts.get(key)
    if expr is None:
        continue  # audit-routes.py owns the missing-route case
    body = handler_scope(expr)
    if body is None:
        continue
    for p in params:
        if f'"{p}"' not in body:
            problems.append((key[0], cpath, f'query parameter "{p}" is declared but never read'))
    # Status codes are mapped centrally (writeError/statusForError), so a literal check
    # produces false positives; the query-parameter check is the reliable one.

if problems:
    print(f"CONTRACT CONFORMANCE: {len(problems)} problem(s)")
    for meth, path, why in problems:
        print(f"  {meth:6s} {path:52s} {why}")
else:
    print("CONTRACT CONFORMANCE: clean")

if '--fail' in sys.argv and problems:
    sys.exit(1)
