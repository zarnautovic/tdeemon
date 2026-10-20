#!/usr/bin/env bash
# Fails on a disable comment in the web app that gives no reason. oxlint
# (unicorn/no-abusive-eslint-disable) makes a disable name its rules; the
# conventions also want the reason after " -- ", as nolintlint wants one
# for Go (docs/conventions.md, "Web app"):
#
#   // oxlint-disable-next-line typescript/no-unsafe-type-assertion -- the API guarantees the shape
#
#   scripts/check-disable-comments.sh           # every file in web/ git does not ignore
#   scripts/check-disable-comments.sh FILE...   # these files (the git hook)
set -euo pipefail
cd "$(dirname "$0")/.."

if (($# > 0)); then
  # Once each: the hook's two globs can both name a file.
  mapfile -t files < <(printf '%s\n' "$@" | sort -u)
else
  # Tracked files and new ones not yet added, but nothing git ignores,
  # such as node_modules or dist.
  mapfile -t files < <(git ls-files --cached --others --exclude-standard -- \
    'web/*.ts' 'web/*.tsx' 'web/*.mts' 'web/*.cts' 'web/*.js' 'web/*.jsx' 'web/*.mjs' 'web/*.cjs')
fi
((${#files[@]} > 0)) || exit 0

# A directive is eslint- or oxlint-disable, -disable-line or
# -disable-next-line; its reason is " -- " and then at least one word.
directive='(eslint|oxlint)-disable(-next-line|-line)?([^a-z-]|$)'
reason='[[:space:]]--[[:space:]]+[[:alnum:]]'

status=0
for file in "${files[@]}"; do
  while IFS= read -r hit; do
    line=${hit#*:}
    if [[ ! $line =~ $reason ]]; then
      echo "$file:${hit%%:*}: a disable comment gives its reason after \" -- \": $line" >&2
      status=1
    fi
  done < <(grep -nE "$directive" -- "$file" || true)
done
exit "$status"
