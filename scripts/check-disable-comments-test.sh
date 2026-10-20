#!/usr/bin/env bash
# Tests scripts/check-disable-comments.sh on made-up files. Run by
# `make test-scripts`.
set -uo pipefail
root=$(cd "$(dirname "$0")/.." && pwd)
tmp=$(mktemp -d)
trap 'rm -rf "$tmp"' EXIT
fails=0

# expect NAME WANT LINE: a file with LINE passes (WANT=pass) or fails.
expect() {
  local name=$1 want=$2 got
  printf 'export const a = 1;\n%s\nexport const b: any = 2;\n' "$3" >"$tmp/probe.ts"
  if "$root/scripts/check-disable-comments.sh" "$tmp/probe.ts" 2>"$tmp/stderr"; then
    got=pass
  else
    got=fail
  fi
  if [[ $got == "$want" ]]; then
    echo "ok   $name: ${got}"
  else
    echo "FAIL $name: want ${want}, got ${got}"
    sed 's/^/     /' "$tmp/stderr"
    fails=$((fails + 1))
  fi
}

expect "a reason after --" pass '// oxlint-disable-next-line typescript/no-explicit-any -- the API guarantees the shape'
expect "eslint-disable with a reason" pass '// eslint-disable-next-line typescript/no-explicit-any -- generated code'
expect "a block comment with a reason" pass '/* eslint-disable react/only-export-components -- shadcn exports variants */'
expect "two rules and a reason" pass '// oxlint-disable-line a/b, c/d -- both are wrong here'
expect "an enable comment" pass '/* eslint-enable react/only-export-components */'
expect "no directive" pass '// a comment that mentions eslint and disable'
expect "no reason" fail '// oxlint-disable-next-line typescript/no-explicit-any'
expect "eslint-disable, no reason" fail '// eslint-disable-next-line typescript/no-explicit-any'
expect "a dash but no reason" fail '// oxlint-disable-next-line typescript/no-explicit-any --'
expect "an empty reason before */" fail '/* eslint-disable react/only-export-components -- */'
expect "a single dash" fail '// oxlint-disable-line typescript/no-explicit-any - reason'
expect "disable-line, no reason" fail '// oxlint-disable-line typescript/no-explicit-any'
expect "a whole-file disable, no reason" fail '/* oxlint-disable typescript/no-explicit-any */'
# The backticks are the text under test, not an expansion.
# shellcheck disable=SC2016
expect "a reason in backticks" pass '// oxlint-disable-next-line typescript/no-explicit-any -- `schema.d.ts` is generated'
expect "a reason in parentheses" pass '// oxlint-disable-next-line a/b -- (see #12) the API'
expect "punctuation only" fail '// oxlint-disable-next-line a/b -- !!!'
expect "a JSX comment, no reason" fail '{/* eslint-disable-next-line react/no-danger */}'
expect "no space after //" fail '//oxlint-disable-next-line a/b'
expect "prose that mentions a disable" pass '// We do not need an eslint-disable here.'
expect "a doc comment" pass '/** eslint-disable-next-line is not a directive here */'
expect "a reason before the directive" fail 'f(" -- x"); // eslint-disable-line a/b'
expect "a second directive without a reason" fail '/* eslint-disable a/b -- why */ x; // eslint-disable-line c/d'
expect "an empty reason mid-line" fail 'foo(); /* eslint-disable-line a/b -- */ bar();'
expect "a block comment's own line, no reason" fail '  eslint-disable no-debugger'
expect "a block comment's own line with a reason" pass '  eslint-disable no-debugger -- a breakpoint for the demo'

# A file named twice, as the hook's two globs can, is reported once.
printf '// oxlint-disable-next-line x/y\n' >"$tmp/twice.ts"
if "$root/scripts/check-disable-comments.sh" "$tmp/twice.ts" "$tmp/twice.ts" 2>"$tmp/stderr"; then
  echo "FAIL a file named twice: it passed"
  fails=$((fails + 1))
elif (($(wc -l <"$tmp/stderr") != 1)); then
  echo "FAIL a file named twice: $(wc -l <"$tmp/stderr") reports, want 1"
  fails=$((fails + 1))
else
  echo "ok   a file named twice: one report"
fi

# A file it cannot read fails, and so does a failing git: neither may
# pass as clean.
if "$root/scripts/check-disable-comments.sh" "$tmp/missing.ts" 2>"$tmp/stderr"; then
  echo "FAIL a missing file: it passed"
  fails=$((fails + 1))
else
  echo "ok   a missing file: fail"
fi
mkdir -p "$tmp/nogit/scripts" "$tmp/nogit/web"
cp "$root/scripts/check-disable-comments.sh" "$tmp/nogit/scripts/"
# A clean file: the run may fail only because git does.
printf '// oxlint-disable-next-line x/y -- a reason\n' >"$tmp/nogit/web/a.ts"
if LC_ALL=C GIT_CEILING_DIRECTORIES=$tmp "$tmp/nogit/scripts/check-disable-comments.sh" 2>"$tmp/stderr"; then
  echo "FAIL outside a git repository: it passed"
  fails=$((fails + 1))
elif ! grep -q 'not a git repository' "$tmp/stderr"; then
  echo "FAIL outside a git repository: it failed for another reason"
  sed 's/^/     /' "$tmp/stderr"
  fails=$((fails + 1))
else
  echo "ok   outside a git repository: fail"
fi

# macOS's /bin/bash is 3.2, which has no mapfile or readarray.
printf 'enable -n mapfile readarray\n' >"$tmp/bash32"
printf '// oxlint-disable-next-line x/y -- a reason\n' >"$tmp/ok.ts"
if BASH_ENV=$tmp/bash32 bash "$root/scripts/check-disable-comments.sh" "$tmp/ok.ts" 2>"$tmp/stderr"; then
  echo "ok   without bash 4 builtins: pass"
else
  echo "FAIL without bash 4 builtins: it failed"
  sed 's/^/     /' "$tmp/stderr"
  fails=$((fails + 1))
fi

# Without arguments it checks the JS and TS files under web/ that git
# does not ignore: tracked ones and new ones, but not node_modules.
mkdir -p "$tmp/repo/scripts" "$tmp/repo/web/src" "$tmp/repo/web/node_modules/pkg"
cp "$root/scripts/check-disable-comments.sh" "$tmp/repo/scripts/"
for file in web/src/tracked.ts web/src/component.tsx web/top.config.ts web/src/čaša.ts web/src/new.ts web/node_modules/pkg/index.js; do
  printf '// oxlint-disable-next-line x/y\nexport const a = 1;\n' >"$tmp/repo/$file"
done
printf 'node_modules/\n' >"$tmp/repo/web/.gitignore"
git -C "$tmp/repo" init -q
git -C "$tmp/repo" add scripts web/.gitignore web/src/tracked.ts web/src/component.tsx web/top.config.ts web/src/čaša.ts
"$tmp/repo/scripts/check-disable-comments.sh" 2>"$tmp/stderr"
rc=$?
missing=
for name in tracked.ts component.tsx top.config.ts čaša.ts new.ts; do
  grep -q "$name" "$tmp/stderr" || missing="$missing $name"
done
if ((rc == 0)) || [[ -n $missing ]]; then
  echo "FAIL the repo's files: not reported:${missing:- (it passed)}"
  sed 's/^/     /' "$tmp/stderr"
  fails=$((fails + 1))
elif grep -q node_modules "$tmp/stderr"; then
  echo "FAIL the repo's files: an ignored file was checked"
  fails=$((fails + 1))
else
  echo "ok   the repo's files: tracked, new, .tsx, top-level and non-ASCII files are checked, ignored ones not"
fi

# A tracked file deleted, or renamed, before the change is staged has
# nothing left to check, and must not fail the run.
mkdir -p "$tmp/gone/scripts" "$tmp/gone/web/src"
cp "$root/scripts/check-disable-comments.sh" "$tmp/gone/scripts/"
printf 'export const a = 1;\n' >"$tmp/gone/web/src/old.ts"
git -C "$tmp/gone" init -q
git -C "$tmp/gone" add scripts web/src/old.ts
mv "$tmp/gone/web/src/old.ts" "$tmp/gone/web/src/renamed.ts"
if "$tmp/gone/scripts/check-disable-comments.sh" 2>"$tmp/stderr"; then
  echo "ok   a tracked file renamed before staging: pass"
else
  echo "FAIL a tracked file renamed before staging: it failed"
  sed 's/^/     /' "$tmp/stderr"
  fails=$((fails + 1))
fi

if ((fails > 0)); then
  echo "$fails failed"
  exit 1
fi
