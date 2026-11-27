#!/usr/bin/env bash
# Boot the API and the four built apps for local inspection.
#
# Why a script: the apps are separate origins on purpose (they are separate deployments), so
# every one needs the API origin baked in at build time and a history fallback for its router.
# Doing this by hand is four builds, four servers and one CORS list that must match.
#
# Usage:
#   scripts/dev-serve.sh          # start everything, print the links
#   scripts/dev-serve.sh stop     # stop everything it started
set -euo pipefail
cd "$(dirname "$0")/.."

PIDDIR=.dev-serve
API_PORT=8099
MAIN_PORT=5180
SHOP_PORT=5191
ADMIN_PORT=5182
MAG_PORT=5193

stop_all() {
  if [ -d "$PIDDIR" ]; then
    for f in "$PIDDIR"/*.pid; do
      [ -e "$f" ] || continue
      kill "$(cat "$f")" 2>/dev/null || true
      rm -f "$f"
    done
    rmdir "$PIDDIR" 2>/dev/null || true
  fi
  echo "stopped"
}

if [ "${1:-}" = "stop" ]; then
  stop_all
  exit 0
fi

if [ -z "${KH_DB_URL:-}" ]; then
  echo "KH_DB_URL is not set. Example:"
  echo '  export KH_DB_URL="postgres://postgres@127.0.0.1:55432/khabavar?sslmode=disable"'
  exit 1
fi
if [ -z "${KH_TOKEN_SIGNING_KEY:-}" ]; then
  echo "KH_TOKEN_SIGNING_KEY is not set."
  exit 1
fi

mkdir -p "$PIDDIR"
ORIGINS="http://127.0.0.1:$MAIN_PORT,http://127.0.0.1:$SHOP_PORT,http://127.0.0.1:$ADMIN_PORT,http://127.0.0.1:$MAG_PORT"

echo "building the apps against http://127.0.0.1:$API_PORT/api/v1 ..."
for app in main shop admin magazine; do
  # npm workspaces live under web/, so the build runs there.
  ( cd web && VITE_API_BASE="http://127.0.0.1:$API_PORT/api/v1" npm run build --workspace "$app" >/dev/null )
  echo "  $app built"
done

echo "building the binary ..."
go build -o /tmp/khabavar ./cmd/khabavar

echo "starting the API on :$API_PORT ..."
KH_ROLE=api KH_HTTP_ADDR=":$API_PORT" \
  KH_OIDC_ISSUER="http://localhost:$API_PORT" \
  KH_CORS_EXTRA_ORIGINS="$ORIGINS" \
  /tmp/khabavar >"$PIDDIR/api.log" 2>&1 &
echo $! >"$PIDDIR/api.pid"

for p in "$MAIN_PORT:main" "$SHOP_PORT:shop" "$ADMIN_PORT:admin" "$MAG_PORT:magazine"; do
  port=${p%%:*}; app=${p##*:}
  python3 scripts/serve-spa.py "web/$app/dist" "$port" >"$PIDDIR/$app.log" 2>&1 &
  echo $! >"$PIDDIR/$app.pid"
done

# Wait for the API, then report. A link that 404s is worse than no link.
for _ in $(seq 1 25); do
  if curl -fsS "http://127.0.0.1:$API_PORT/healthz" >/dev/null 2>&1; then break; fi
  sleep 0.4
done

echo
echo "  API        http://127.0.0.1:$API_PORT/healthz"
echo "  main       http://127.0.0.1:$MAIN_PORT/"
echo "  shop       http://127.0.0.1:$SHOP_PORT/"
echo "  admin      http://127.0.0.1:$ADMIN_PORT/"
echo "  magazine   http://127.0.0.1:$MAG_PORT/"
echo
echo "  panel routes: #/orders  #/products  #/stock  #/returns  #/audit  #/roles"
echo "  shop routes : #/cart    #/checkout"
echo
echo "  stop with: scripts/dev-serve.sh stop"
