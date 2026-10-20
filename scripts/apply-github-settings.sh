#!/usr/bin/env bash
# Applies the GitHub settings kept in the repo: the merge settings and the
# ruleset for main (.github/rulesets/main.json). It needs gh logged in
# with admin rights on the repo, and the required checks must already run
# on pull requests, or nothing could be merged.
#
#   scripts/apply-github-settings.sh             # apply
#   scripts/apply-github-settings.sh --dry-run   # show what would change
set -euo pipefail

repo="${GITHUB_REPOSITORY:-zarnautovic/tdeemon}"
ruleset=.github/rulesets/main.json
dry_run=false
if [[ "${1:-}" == "--dry-run" ]]; then
  dry_run=true
fi

# Writes go through here, so --dry-run prints them instead.
write() {
  if "$dry_run"; then
    echo "would run: gh api $*"
  else
    gh api "$@"
  fi
}

# Squash merges only, with the pull request's title as the commit (the
# release tooling reads it); merged branches are deleted.
write --method PATCH "repos/$repo" --silent \
  -F allow_squash_merge=true \
  -F allow_merge_commit=false \
  -F allow_rebase_merge=false \
  -f squash_merge_commit_title=PR_TITLE \
  -f squash_merge_commit_message=PR_BODY \
  -F delete_branch_on_merge=true

name=$(jq -r .name "$ruleset")
id=$(gh api "repos/$repo/rulesets" --jq ".[] | select(.name == \"$name\") | .id")
if [[ -n "$id" ]]; then
  write --method PUT "repos/$repo/rulesets/$id" --silent --input "$ruleset"
else
  write --method POST "repos/$repo/rulesets" --silent --input "$ruleset"
fi
