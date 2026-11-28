#!/usr/bin/env python3
"""Check the nginx routing against what each role actually mounts.

Why this exists: deploy/nginx/khabavar.conf decides which unit answers a request, and nothing has
ever compared that decision with the route tables. nginx is not installed on the build machine, so
the config cannot be exercised — but the mapping it encodes can still be checked statically, and
that is where the mistakes are (a route sent to identity that identity does not serve answers 404
in production and works everywhere else).

Usage:
  python3 scripts/audit-nginx.py            # report
  python3 scripts/audit-nginx.py --fail     # exit 1 on a mismatch
"""
import glob
import os
import re
import sys

ROOT = os.path.dirname(os.path.dirname(os.path.abspath(__file__)))
CONF = os.path.join(ROOT, "deploy", "nginx", "khabavar.conf")
IDENTITY_ROUTES = os.path.join(ROOT, "cmd", "khabavar", "identity_routes.go")

LOCATION_RE = re.compile(r"location\s+(\S+)\s*\{([^}]*)\}", re.S)
PROXY_RE = re.compile(r"proxy_pass\s+http://([a-z_]+)")
PATTERN_RE = re.compile(r'"([A-Z]+) (/[^"]*)"')


def nginx_locations():
    """[(prefix, upstream)] in file order — nginx matches longest prefix first."""
    with open(CONF, encoding="utf-8") as f:
        conf = f.read()
    out = []
    for prefix, body in LOCATION_RE.findall(conf):
        m = PROXY_RE.search(body)
        out.append((prefix, m.group(1) if m else "static"))
    return out


def identity_patterns():
    with open(IDENTITY_ROUTES, encoding="utf-8") as f:
        return [p for _, p in PATTERN_RE.findall(f.read())]


def upstream_for(path, locations):
    """Longest matching prefix wins, the same rule nginx applies."""
    best = ("/", "static")
    for prefix, upstream in locations:
        if prefix == "/":
            continue
        if path.startswith(prefix) and len(prefix) > len(best[0]):
            best = (prefix, upstream)
    return best[1]


def main():
    fail = "--fail" in sys.argv
    locations = nginx_locations()
    identity = identity_patterns()

    print(f"nginx locations: {len(locations)}")
    for prefix, up in locations:
        print(f"  {prefix:20} -> {up}")

    problems = []

    # 1) Every route the identity unit mounts must be routed to identity, or it 404s in
    #    production while working on a single-origin dev box.
    for path in identity:
        up = upstream_for(path, locations)
        if up != "khabavar_identity":
            problems.append(f"identity mounts {path} but nginx routes it to {up}")

    # 2) The storefront must not be sent to identity: identity does not serve it.
    for probe in ("/api/v1/catalog/products", "/api/v1/orders", "/api/v1/payments/callback"):
        up = upstream_for(probe, locations)
        if up != "khabavar_api":
            problems.append(f"{probe} is routed to {up}, expected khabavar_api")

    # 3) Static apps need a history fallback, or every client route 404s on refresh.
    with open(CONF, encoding="utf-8") as f:
        conf = f.read()
    servers = conf.count("server_name")
    fallbacks = conf.count("try_files $uri $uri/ /index.html")
    if fallbacks < servers:
        problems.append(f"{servers} server blocks but only {fallbacks} SPA fallbacks")

    if not problems:
        print(f"\nNGINX ROUTING: clean ({len(identity)} identity routes, {servers} servers)")
        return 0

    print(f"\nNGINX ROUTING MISMATCHES ({len(problems)}):")
    for p in problems:
        print(f"  {p}")
    return 1 if fail else 0


if __name__ == "__main__":
    sys.exit(main())
