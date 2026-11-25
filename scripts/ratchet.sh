#!/usr/bin/env bash
# The ratchet: checkers may get stricter, never weaker, and nobody has to
# approve the change by hand.
#
#   scripts/ratchet.sh BASE
#
# BASE is the commit being built on (main for a pull request, the previous
# commit for a push). Two checks:
#   1. BASE's checker tests run against the new checker code, so every
#      control BASE had (good certificates accepted, damaged ones rejected)
#      still holds.
#   2. Every claim is re-graded with BASE's checkers, so a change cannot
#      weaken the judge and grade its own claims in the same step.
set -euo pipefail
base=${1:?usage: scripts/ratchet.sh BASE}
if ! git cat-file -e "$base^{commit}" 2>/dev/null; then
  echo "ratchet: base $base not found; nothing to compare (first commit?)"
  exit 0
fi
root=$(git rev-parse --show-toplevel)
work=$(mktemp -d)
trap 'git -C "$root" worktree remove --force "$work/new" >/dev/null 2>&1 || true; rm -rf "$work"' EXIT
git -C "$root" worktree add -q --detach "$work/new" HEAD
cd "$work/new"
# Uncommitted changes are part of what is being checked.
git -C "$root" diff HEAD | git apply --allow-empty 2>/dev/null || true

checkers=$(git ls-tree -d --name-only "$base" cmd/ | grep -E '^cmd/(verify|check)-' || true)

echo "== 1. base controls against new checkers"
for d in $checkers; do
  if [ ! -d "$d" ]; then
    echo "FAIL: checker $d was removed"
    exit 1
  fi
  for t in $(git ls-tree --name-only "$base" "$d/" | grep '_test\.go$'); do
    git show "$base:$t" > "$t"
  done
  go test -count=1 "./$d" >/dev/null || { echo "FAIL: $d no longer passes the base controls"; go test -count=1 "./$d" | tail -20; exit 1; }
  echo "ok $d"
done
git checkout -q -- .

echo "== 2. claims graded by base checkers"
for d in $checkers; do
  git checkout -q "$base" -- "$d"
done
go run ./cmd/wb claims -run
