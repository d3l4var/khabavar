#!/usr/bin/env bash
# The same a11y sweep CI runs, so it can be reproduced locally in one command.
# Requires: the four apps built (web/*/dist) and lightpanda on PATH.
set -euo pipefail
cd "$(dirname "$0")/.."
for app in main shop admin magazine; do
  node scripts/a11y-audit.mjs "web/$app/dist" /
done
node scripts/a11y-audit.mjs web/shop/dist "/#/cart"
node scripts/a11y-audit.mjs web/shop/dist "/#/checkout"
node scripts/a11y-audit.mjs web/admin/dist "/#/returns"
node scripts/a11y-audit.mjs web/admin/dist "/#/stock"
node scripts/a11y-audit.mjs web/magazine/dist "/#/topics"
echo "a11y: all routes clean"
