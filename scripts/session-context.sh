#!/usr/bin/env bash
# SessionStart hook: stdout is added to Claude's context so a new session knows where the work stands.
set -euo pipefail
cd "${CLAUDE_PROJECT_DIR:-$(git rev-parse --show-toplevel)}"

echo "# road-to-knowledge · session context (SessionStart hook)"
echo
awk '/^## Start here/{on=1} /^## Log/{on=0} on' docs/PROGRESS.md

echo "## Board"
project="$(gh project list --owner TheGostsniperfr --format json 2>/dev/null | jq -r '.projects[] | select(.title=="Road to Knowledge") | .number' 2>/dev/null || true)"
if [[ -n "$project" ]]; then
  gh project item-list "$project" --owner TheGostsniperfr --format json --limit 100 2>/dev/null \
    | jq -r '.items[] | select(.status != "Done") | "- [\(.status // "Backlog")] \(.title)"' | sort | head -12
else
  echo "- board not reachable (gh auth refresh -s project to grant access)"
fi
echo
echo "## Git"
echo "- branch $(git branch --show-current), last commit: $(git log -1 --format='%h %s (%cr)' 2>/dev/null || echo none)"
dirty="$(git status --porcelain | wc -l)"
if [[ "$dirty" -gt 0 ]]; then
  echo "- $dirty uncommitted change(s): a previous session stopped mid-work, check them first"
fi
prs="$(gh pr list --state open --json number,title -q '.[] | "- PR #\(.number) \(.title)"' 2>/dev/null || true)"
[[ -n "$prs" ]] && { echo "## Open PRs"; echo "$prs"; }
echo
echo "Protocol: AGENTS.md. Run make init && make check before changing code."
