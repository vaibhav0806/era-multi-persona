#!/usr/bin/env bash
# M0 dummy runner. Clones sandbox repo, makes trivial change, pushes new branch.
# INPUT (env vars):
#   PI_TASK_ID          task id (numeric)
#   PI_TASK_DESCRIPTION plain text description
#   PI_GITHUB_PAT       GitHub PAT with repo scope on sandbox repo
#   PI_GITHUB_REPO      owner/repo
# OUTPUT:
#   stdout last line: "RESULT branch=<name> summary=<text>"  (on success)
#   stderr: all logs
#   exit 0 on success, non-zero on failure.

set -euo pipefail

: "${PI_TASK_ID:?PI_TASK_ID required}"
: "${PI_TASK_DESCRIPTION:?PI_TASK_DESCRIPTION required}"
: "${PI_GITHUB_PAT:?PI_GITHUB_PAT required}"
: "${PI_GITHUB_REPO:?PI_GITHUB_REPO required}"

branch="agent/${PI_TASK_ID}/dummy-$(date -u +%s)"
work=$(mktemp -d)
cd "$work"

git config --global user.email "era@local"
git config --global user.name  "era"
git config --global advice.detachedHead false

git clone --depth 1 "https://x-access-token:${PI_GITHUB_PAT}@github.com/${PI_GITHUB_REPO}.git" repo
cd repo
git checkout -b "$branch"

{
  echo ""
  echo "## Task #${PI_TASK_ID}"
  echo "${PI_TASK_DESCRIPTION}"
  echo ""
  echo "_Dummy commit from M0 runner at $(date -u +%FT%TZ)._"
} >> README.md

git add README.md
git commit -m "task #${PI_TASK_ID}: ${PI_TASK_DESCRIPTION}"
git push origin "$branch"

echo "RESULT branch=${branch} summary=dummy-commit-ok"
