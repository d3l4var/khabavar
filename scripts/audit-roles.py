#!/usr/bin/env python3
"""Boot every KH_ROLE against a real database and assert each one is alive and isolated.

Why this exists: three separate defects shipped because only KH_ROLE=api had ever been run —
staff login, customer OTP and OIDC were each mounted in one unit only, and the identity unit
finally stopped booting entirely on a duplicate route pattern. A unit that cannot start is not
caught by unit tests, because the panic happens in main().

Each role is started, /readyz must answer 200, then a small set of routes that MUST exist and
routes that MUST NOT exist are probed. Any failure exits 1.

Usage:
  KH_TEST_PG_DSN=postgres://... python3 scripts/audit-roles.py [--binary /tmp/khabavar]
"""
import argparse
import json
import os
import subprocess
import sys
import time
import urllib.error
import urllib.request

# (role, port, required paths, forbidden paths)
# required: (method, path) that must not 404/405
# forbidden: (method, path) that must 404 — the isolation guarantee between units
ROLES = [
    (
        "api",
        18081,
        [
            ("GET", "/healthz"),
            ("GET", "/api/v1/products/search?q=x"),
            ("POST", "/api/v1/auth/otp/request"),
            ("POST", "/api/v1/auth/admin/login"),
            ("GET", "/api/v1/auth/admin/login?continuation=x"),
            ("GET", "/oauth/authorize?response_type=code&client_id=khabavar-admin"
                    "&redirect_uri=http%3A%2F%2F127.0.0.1%3A5180%2Fauth%2Fcallback"
                    "&scope=openid&code_challenge=x&code_challenge_method=S256"),
        ],
        [],
    ),
    (
        "identity",
        18082,
        [
            ("GET", "/healthz"),
            ("POST", "/api/v1/auth/otp/request"),
            ("GET", "/api/v1/auth/otp/login?continuation=x"),
            ("GET", "/api/v1/auth/admin/login?continuation=x"),
            ("POST", "/api/v1/auth/admin/login"),
            ("GET", "/oauth/authorize?response_type=code&client_id=khabavar-admin"
                    "&redirect_uri=http%3A%2F%2F127.0.0.1%3A5180%2Fauth%2Fcallback"
                    "&scope=openid&code_challenge=x&code_challenge_method=S256"),
            ("POST", "/oauth/token"),
        ],
        # The identity unit must not serve the storefront.
        [("GET", "/api/v1/products/search?q=x")],
    ),
    (
        "worker",
        18083,
        [("GET", "/healthz")],
        [
            # The worker must not serve the API surface at all.
            ("GET", "/api/v1/products/search?q=x"),
            ("POST", "/api/v1/auth/admin/login"),
        ],
    ),
]


def probe(port, method, path, timeout=5):
    url = f"http://127.0.0.1:{port}{path}"
    data = b"{}" if method == "POST" else None
    req = urllib.request.Request(url, data=data, method=method,
                                 headers={"Content-Type": "application/json"})
    try:
        with urllib.request.urlopen(req, timeout=timeout) as r:
            return r.status
    except urllib.error.HTTPError as e:
        return e.code
    except Exception:
        return 0


def wait_ready(port, proc, deadline=25):
    """Wait for /healthz. A role that panics exits; report that instead of timing out blindly."""
    start = time.time()
    while time.time() - start < deadline:
        if proc.poll() is not None:
            return False
        if probe(port, "GET", "/healthz", timeout=2) == 200:
            return True
        time.sleep(0.3)
    return False


def main():
    ap = argparse.ArgumentParser()
    ap.add_argument("--binary", default=os.environ.get("KH_BINARY", "/tmp/khabavar"))
    args = ap.parse_args()

    dsn = os.environ.get("KH_TEST_PG_DSN") or os.environ.get("KH_DB_URL")
    if not dsn:
        print("KH_TEST_PG_DSN (or KH_DB_URL) is required: the roles need a database")
        return 2
    if not os.path.exists(args.binary):
        print(f"binary not found: {args.binary} (build it first: go build -o {args.binary} ./cmd/khabavar)")
        return 2

    failures = []
    for role, port, required, forbidden in ROLES:
        env = dict(os.environ)
        env.update({
            "KH_ROLE": role,
            "KH_DB_URL": dsn,
            "KH_TOKEN_SIGNING_KEY": "role-audit-signing-key-0123456789",
            "KH_HTTP_ADDR": f":{port}",
            "KH_OIDC_ISSUER": f"http://localhost:{port}",
        })
        proc = subprocess.Popen([args.binary], env=env,
                                stdout=subprocess.PIPE, stderr=subprocess.STDOUT)
        try:
            if not wait_ready(port, proc):
                out = b""
                if proc.poll() is not None:
                    out = proc.stdout.read() or b""
                text = out.decode("utf8", "replace").strip().splitlines()
                head = text[0] if text else "(no output)"
                failures.append(f"{role}: did not become ready — {head}")
                print(f"FAIL {role:9} did not become ready — {head}")
                continue

            bad = []
            for method, path in required:
                code = probe(port, method, path)
                if code in (0, 404, 405):
                    bad.append(f"required {method} {path} -> {code}")
            for method, path in forbidden:
                code = probe(port, method, path)
                if code not in (404, 405):
                    bad.append(f"forbidden {method} {path} -> {code} (must not be served here)")

            if bad:
                failures.append(f"{role}: " + "; ".join(bad))
                print(f"FAIL {role:9} " + "; ".join(bad))
            else:
                print(f"ok   {role:9} ready on :{port}, {len(required)} required, "
                      f"{len(forbidden)} forbidden")
        finally:
            proc.terminate()
            try:
                proc.wait(timeout=10)
            except subprocess.TimeoutExpired:
                proc.kill()

    if failures:
        print(f"\nROLE AUDIT: {len(failures)} failing role(s)")
        return 1
    print("\nROLE AUDIT: all three roles boot and stay isolated")
    return 0


if __name__ == "__main__":
    sys.exit(main())
