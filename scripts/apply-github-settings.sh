#!/usr/bin/env bash
# Applies the GitHub settings kept in the repo: the merge settings and the
# ruleset for main (.github/rulesets/main.json). It needs gh logged in
# with admin rights on the repo, and jq; the required checks must already
# run on pull requests, or nothing could be merged.
#
#   scripts/apply-github-settings.sh             # apply
#   scripts/apply-github-settings.sh --dry-run   # print the writes instead
set -euo pipefail

dry_run=false
case "$#:${1:-}" in
  0:) ;;
  1:--dry-run) dry_run=true ;;
  *)
    echo "usage: $0 [--dry-run]" >&2
    exit 2
    ;;
esac

# Paths are relative to the repository root, wherever this is run from.
cd "$(dirname "$0")/.."
repo="${GITHUB_REPOSITORY:-zarnautovic/tdeemon}"
ruleset=.github/rulesets/main.json

# Everything is read and checked before the first write, so a failure
# leaves the settings as they were.
command -v jq >/dev/null || { echo "jq is needed" >&2; exit 1; }
name=$(jq -er .name "$ruleset")
id=$(gh api "repos/$repo/rulesets" --jq ".[] | select(.name == \"$name\") | .id")

# Writes go through here, so --dry-run prints them instead.
write() {
  if "$dry_run"; then
    echo "would run: gh api $*"
  else
    gh api "$@"
  fi
}

# Squash merges only, with the pull request's title as the commit (the
# release tooling reads it); merged branches are deleted; auto-merge lets
# Renovate merge its updates once the required checks pass.
write --method PATCH "repos/$repo" --silent \
  -F allow_auto_merge=true \
  -F allow_squash_merge=true \
  -F allow_merge_commit=false \
  -F allow_rebase_merge=false \
  -f squash_merge_commit_title=PR_TITLE \
  -f squash_merge_commit_message=PR_BODY \
  -F delete_branch_on_merge=true

if [[ -n "$id" ]]; then
  write --method PUT "repos/$repo/rulesets/$id" --silent --input "$ruleset"
else
  write --method POST "repos/$repo/rulesets" --silent --input "$ruleset"
fi
