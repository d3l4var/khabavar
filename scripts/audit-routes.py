#!/usr/bin/env python3
"""Diff the routes the binary mounts against the paths the contract declares.

Why: a route that exists but is undocumented is invisible to clients; a documented path
that is not mounted is a promise the API breaks at runtime. Both were found by hand
before (GET /admin/products), so this is a gate, not a report.

Usage:  python3 scripts/audit-routes.py [--fail]
"""
import re
import sys

# Every file under cmd/khabavar, not a fixed list: moving a route table into its own file made
# this audit blind to those routes and reported a false "declared but not mounted".
import glob
ROUTES = sorted(f for f in glob.glob('cmd/khabavar/*.go') if not f.endswith('_test.go'))
CONTRACT = 'api/openapi.yaml'
PREFIX = '/api/v1'


def norm(path: str) -> str:
    # {id} in Go mux and {id} in OpenAPI are the same shape; normalise the name only.
    return re.sub(r'\{[^}]+\}', '{}', path.rstrip('/')) or '/'


mounted = set()
for f in ROUTES:
    src = open(f, encoding='utf-8').read()
    for m in re.finditer(r'"(GET|POST|PUT|PATCH|DELETE) (/[^"]+)"', src):
        method, path = m.group(1), m.group(2)
        # Root-level routes (/oauth/*, /healthz) are part of the contract too.
        mounted.add((method, norm(path[len(PREFIX):] if path.startswith(PREFIX) else path)))

contract = set()
src = open(CONTRACT, encoding='utf-8').read()
for m in re.finditer(r'^  (/[^:]+):\s*$', src, re.M):
    path = m.group(1)
    body_start = m.end()
    nxt = re.search(r'^  /[^:]+:\s*$', src[body_start:], re.M)
    body = src[body_start: body_start + (nxt.start() if nxt else len(src))]
    for mm in re.finditer(r'^\s{4}(get|post|put|patch|delete):\s*$', body, re.M):
        contract.add((mm.group(1).upper(), norm(path)))

only_code = sorted(mounted - contract)
only_contract = sorted(contract - mounted)

print(f"mounted: {len(mounted)} | declared: {len(contract)}")
if only_code:
    print("\nMOUNTED BUT NOT IN THE CONTRACT (clients cannot discover these):")
    for m, p in only_code:
        print(f"  {m:6s} {PREFIX}{p}")
if only_contract:
    print("\nDECLARED BUT NOT MOUNTED (the API breaks this promise):")
    for m, p in only_contract:
        print(f"  {m:6s} {p}")
if not only_code and not only_contract:
    print("\nroutes and contract agree")
# --fail is the hard promise: a documented path that is not mounted breaks clients.
# --fail-undocumented is the reverse gate, opt-in because closing all of them today is a
# contract revision that needs owner approval.
if '--fail' in sys.argv and only_contract:
    sys.exit(1)
if '--fail-undocumented' in sys.argv and only_code:
    sys.exit(1)
sys.exit(0)
