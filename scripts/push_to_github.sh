#!/usr/bin/env bash
# push_to_github.sh — pushes all 8 local commits to d3l4var/khabavar:main
#
# The agent prepared 8 commits (5 module source + 3 docs) on the `github-sync`
# branch, sitting on top of origin/main. This script sends them upstream.
#
# USAGE (pick ONE):
#
#   # Option A: Personal Access Token (recommended)
#   GH_TOKEN=ghp_your_token_here bash scripts/push_to_github.sh
#
#   # Option B: gh CLI (if installed + authenticated)
#   gh auth login   # one-time
#   bash scripts/push_to_github.sh
#
#   # Option C: already-configured git credentials
#   bash scripts/push_to_github.sh
#
# The token needs 'repo' scope (or fine-grained: Contents read+write on the repo).
# Create one at: https://github.com/settings/tokens
#
set -euo pipefail
cd "$(git rev-parse --show-toplevel)"

REMOTE_URL="https://github.com/d3l4var/khabavar.git"

echo "============================================"
echo "  Pushing khabavar commits to GitHub"
echo "============================================"
echo ""
echo "Branch: $(git branch --show-current)"
echo "Commits to push:"
git log --oneline origin/main..github-sync 2>/dev/null || git log --oneline -8
echo ""
echo "Files changed: $(git diff --stat origin/main..github-sync 2>/dev/null | tail -1)"
echo ""

# --- authenticate ---
if [ -n "${GH_TOKEN:-}" ]; then
  echo ">> Using GH_TOKEN for authentication..."
  git push "https://x-access-token:${GH_TOKEN}@github.com/d3l4var/khabavar.git" github-sync:main
elif command -v gh >/dev/null 2>&1 && gh auth status >/dev/null 2>&1; then
  echo ">> Using gh CLI authentication..."
  gh auth setup-git
  git push origin github-sync:main
else
  echo ">> Attempting push with existing git credentials..."
  git push origin github-sync:main
fi

PUSH_EXIT=$?

if [ $PUSH_EXIT -eq 0 ]; then
  echo ""
  echo "Push successful!"
  echo ""
  echo "Verify at: https://github.com/d3l4var/khabavar"
  echo ""
  echo "Fetching updated remote..."
  git fetch origin
  git log --oneline origin/main -8
else
  echo ""
  echo "Push failed (exit $PUSH_EXIT)."
  echo ""
  echo "If you see 'could not read Username', provide a token:"
  echo "  GH_TOKEN=ghp_xxx bash scripts/push_to_github.sh"
  echo ""
  echo "Create a token at: https://github.com/settings/tokens"
  echo "(needs 'repo' scope, or fine-grained Contents read+write on d3l4var/khabavar)"
  exit $PUSH_EXIT
fi
