#!/usr/bin/env bash
# Run the CI gates locally, in the same order the workflow runs them.
#
# Why this exists: seven of the CI jobs are blocking and the workflow has never executed on a
# GitHub runner. A gate that has never run is not a gate. This script runs the same commands on
# this machine so a failure surfaces here rather than on the first push.
#
# It is NOT a substitute for CI: it cannot prove the runner image has the tools, the Postgres
# service comes up, or the a11y job can download Lightpanda. It proves the commands themselves.
#
# Usage:
#   scripts/ci-local.sh              # every gate that can run here
#   scripts/ci-local.sh --fast       # skip the slow ones (race, integration, concurrency)
set -uo pipefail
cd "$(dirname "$0")/.."

FAST=0
[ "${1:-}" = "--fast" ] && FAST=1

PASS=0
FAIL=0
SKIP=0
FAILED=()

gate() {
  local name="$1"; shift
  printf '\n── %s\n' "$name"
  if "$@"; then
    printf '   PASS\n'
    PASS=$((PASS + 1))
  else
    printf '   FAIL\n'
    FAIL=$((FAIL + 1))
    FAILED+=("$name")
  fi
}

skip() {
  printf '\n── %s\n   SKIP (%s)\n' "$1" "$2"
  SKIP=$((SKIP + 1))
}

# ── static ────────────────────────────────────────────────────────────────────
gate "go vet" go vet ./...
gate "gofmt" bash -c '[ -z "$(gofmt -l .)" ] || { gofmt -l .; false; }'
gate "go build" go build ./...

# ── config and contract ───────────────────────────────────────────────────────
gate "env surface" python3 scripts/audit-env.py --fail-on-missing
gate "routes vs contract" python3 scripts/audit-routes.py --fail --fail-undocumented
gate "path params" python3 scripts/audit-pathparams.py --fail
gate "nginx routing" python3 scripts/audit-nginx.py --fail
gate "contract conformance" python3 scripts/audit-contract-conformance.py --fail
gate "coverage thresholds" python3 scripts/audit-coverage.py --fail

# ── tests ─────────────────────────────────────────────────────────────────────
if [ "$FAST" = "1" ]; then
  skip "go test -race" "--fast"
else
  gate "go test -race" go test -race -count=1 ./...
fi

if [ -n "${KH_TEST_PG_DSN:-}" ]; then
  gate "integration (PG)" go test -count=1 ./...
else
  skip "integration (PG)" "KH_TEST_PG_DSN is not set"
fi

# ── lint ──────────────────────────────────────────────────────────────────────
LINT="$(go env GOPATH)/bin/golangci-lint"
if [ -x "$LINT" ]; then
  gate "golangci-lint" "$LINT" run --timeout=5m ./...
else
  skip "golangci-lint" "not installed (go install github.com/golangci/golangci-lint/v2/cmd/golangci-lint@v2.15.0)"
fi

# ── frontend ──────────────────────────────────────────────────────────────────
if [ -d web/node_modules ]; then
  gate "web typecheck" bash -c 'cd web && for a in main shop admin magazine; do npx tsc --noEmit -p "$a" || exit 1; done'
  gate "a11y" bash scripts/a11y-all.sh
else
  skip "web typecheck / a11y" "web/node_modules is missing (npm install)"
fi

# ── runtime gates ─────────────────────────────────────────────────────────────
if [ -x /tmp/khabavar ] && [ -n "${KH_TEST_PG_DSN:-}" ]; then
  gate "role audit" env KH_BINARY=/tmp/khabavar python3 scripts/audit-roles.py
else
  skip "role audit" "needs /tmp/khabavar and KH_TEST_PG_DSN"
fi

printf '\n════════════════════════════════════════\n'
printf 'pass %d · fail %d · skip %d\n' "$PASS" "$FAIL" "$SKIP"
if [ "$FAIL" -gt 0 ]; then
  printf 'failed: %s\n' "${FAILED[*]}"
  exit 1
fi
printf 'CI-LOCAL: clean\n'
