#!/usr/bin/env bash
# Fails on a disable comment in the web app that gives no reason. oxlint
# (unicorn/no-abusive-eslint-disable) makes a disable name its rules; the
# conventions also want the reason after " -- ", on the same line, as
# nolintlint wants one for Go (docs/conventions.md, "Web app"):
#
#   // oxlint-disable-next-line typescript/no-unsafe-type-assertion -- the API guarantees the shape
#
#   scripts/check-disable-comments.sh           # every JS/TS file in web/ that git does not ignore
#   scripts/check-disable-comments.sh FILE...   # these files (the git hook)
#
# It runs on macOS's bash 3.2 too, so no mapfile.
set -euo pipefail
cd "$(dirname "$0")/.."

if (($# > 0)); then
  # Once each: the hook's two globs can both name a file.
  list=$(printf '%s\n' "$@" | sort -u)
else
  # Tracked files and new ones not yet added, but nothing git ignores,
  # such as node_modules or dist; once each, as a merge conflict lists a
  # file per stage. A plain assignment, so that a failing git stops the
  # script.
  list=$(git ls-files --cached --others --exclude-standard -- \
    'web/*.ts' 'web/*.tsx' 'web/*.mts' 'web/*.cts' 'web/*.js' 'web/*.jsx' 'web/*.mjs' 'web/*.cjs' |
    sort -u)
fi
files=()
while IFS= read -r file; do
  # A tracked file deleted or renamed but not yet staged is still listed;
  # it has nothing to check. A file named on the command line must exist.
  if [[ -n $file ]] && { (($# > 0)) || [[ -e $file ]]; }; then
    files+=("$file")
  fi
done <<<"$list"
((${#files[@]} > 0)) || exit 0

# A directive opens a comment, or a line of a block comment: eslint- or
# oxlint-disable, -disable-line or -disable-next-line. Its reason is
# " -- " and then a word, which may follow punctuation such as a
# backtick or a parenthesis, after the directive on the same line.
directive='(^[[:space:]]*|//|/\*)[[:space:]]*(eslint|oxlint)-disable(-next-line|-line)?([^a-z-]|$)'
reason='[[:space:]]--[[:space:]]+[^[:alnum:]]*[[:alnum:]]'

status=0
for file in "${files[@]}"; do
  if hits=$(grep -anE "$directive" -- "$file"); then
    while IFS= read -r hit; do
      line=${hit#*:}
      # The text after the line's last directive, up to the end of its
      # block comment if it is in one.
      after=${line##*lint-disable}
      after=${after%%\*/*}
      if [[ ! $after =~ $reason ]]; then
        echo "$file:${hit%%:*}: a disable comment gives its reason after \" -- \", on the same line: $line" >&2
        status=1
      fi
    done <<<"$hits"
  elif (($? > 1)); then
    # grep has said why it could not read the file.
    status=1
  fi
done
exit "$status"
