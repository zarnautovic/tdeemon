#!/usr/bin/env bash
# Tests scripts/next-version.sh against the release rules in
# docs/conventions.md ("Releases"), and the release notes template, each
# case in a scratch repository with this repository's cog.toml. Run by
# `make test-scripts`.
set -uo pipefail
root=$(cd "$(dirname "$0")/.." && pwd)
tmp=$(mktemp -d)
trap 'rm -rf "$tmp"' EXIT
fails=0

# repo: a fresh repository with the script, cog.toml and one commit.
repo() {
  local dir
  dir=$(mktemp -d -p "$tmp")
  cd "$dir" || exit 1
  git init -q
  git config user.name test
  git config user.email test@example.com
  # A contributor's global signing would need a key, or a tag message.
  git config commit.gpgsign false
  git config tag.gpgsign false
  mkdir scripts
  cp "$root/scripts/next-version.sh" scripts/
  cp "$root/cog.toml" .
  git add -A
  git commit -q -m "chore: init"
}

commit() {
  git commit -q --allow-empty "$@"
}

# expect NAME WANT: the script prints WANT ("" for no release), or
# fails when WANT is "error".
expect() {
  local name=$1 want=$2 got rc
  got=$(scripts/next-version.sh 2>"$tmp/stderr")
  rc=$?
  if [[ $want == error ]]; then
    if ((rc != 0)); then
      echo "ok   $name: fails"
      return
    fi
  elif ((rc == 0)) && [[ $got == "$want" ]]; then
    echo "ok   $name: ${got:-no release}"
    return
  fi
  echo "FAIL $name: want ${want:-no release}, got ${got:-nothing} (exit $rc)"
  sed 's/^/     /' "$tmp/stderr"
  fails=$((fails + 1))
}

# case NAME WANT TAG MESSAGE...: one commit after TAG ("-" for none).
case_() {
  local name=$1 want=$2 tag=$3
  shift 3
  repo
  if [[ $tag != - ]]; then
    commit -m "feat: base"
    git tag "$tag"
  fi
  commit "$@"
  expect "$name" "$want"
}

case_ "no tag, chore" "" - -m "chore: x"
case_ "no tag, feat" v0.1.0 - -m "feat: x"
case_ "no tag, fix" v0.0.1 - -m "fix: x"
case_ "no tag, fix!" v0.1.0 - -m "fix!: x"
case_ "no tag, refactor!" v0.1.0 - -m "refactor!: x"
case_ "no tag, docs with a footer" v0.1.0 - -m "docs: x" -m "BREAKING CHANGE: y"

case_ "v0.1.0, fix" v0.1.1 v0.1.0 -m "fix: x"
case_ "v0.1.0, perf" v0.1.1 v0.1.0 -m "perf: x"
case_ "v0.1.0, feat" v0.2.0 v0.1.0 -m "feat: x"
case_ "v0.1.0, feat!" v0.2.0 v0.1.0 -m "feat!: x"
case_ "v0.1.0, fix!" v0.2.0 v0.1.0 -m "fix!: x"
case_ "v0.1.0, perf!" v0.2.0 v0.1.0 -m "perf!: x"
case_ "v0.1.0, refactor!" v0.2.0 v0.1.0 -m "refactor!: x"
case_ "v0.1.0, docs!" v0.2.0 v0.1.0 -m "docs!: x"
case_ "v0.1.0, fix(store)!" v0.2.0 v0.1.0 -m "fix(store)!: x"
case_ "v0.1.0, fix with a footer" v0.2.0 v0.1.0 -m "fix: x" -m "BREAKING CHANGE: y"
case_ "v0.1.0, docs with a footer" v0.2.0 v0.1.0 -m "docs: x" -m "BREAKING-CHANGE: y"
for type in docs refactor test ci build chore style; do
  case_ "v0.1.0, $type" "" v0.1.0 -m "$type: x"
done

case_ "v1.0.0, fix" v1.0.1 v1.0.0 -m "fix: x"
case_ "v1.0.0, feat" v1.1.0 v1.0.0 -m "feat: x"
case_ "v1.0.0, fix!" v2.0.0 v1.0.0 -m "fix!: x"
case_ "v1.0.0, refactor!" v2.0.0 v1.0.0 -m "refactor!: x"
case_ "v1.0.0, docs" "" v1.0.0 -m "docs: x"

repo
commit -m "feat: x"
git tag v0.1.0
expect "a re-run on a released commit" ""

repo
commit -m "fix: older"
commit -m "feat: newer"
git tag v0.2.0
git checkout -q HEAD~1
expect "an older commit after a newer release" ""

repo
commit -m "feat: x"
touch untracked
expect "untracked files" v0.1.0

repo
commit -m "feat: x"
git tag v0.1.0
commit -m "fix!: y"
git tag v0.2.0
commit -m "docs: z"
expect "a breaking change behind the last tag, then docs" ""
commit -m "fix: w"
expect "a breaking change behind the last tag, then fix" v0.2.1

repo
commit -m "fix: x"
git tag vnext
expect "a tag that is not a version on HEAD" v0.0.1

repo
echo 'tag_prefix =' >>cog.toml
git commit -q -am "feat: x"
expect "a broken cog.toml" error

# A cog that fails, or prints something unexpected, in one subcommand
# and runs the real one otherwise: the script must fail, not report
# that there is nothing to release.
real_cog=$(command -v cog)
mkdir "$tmp/stub"
cat >"$tmp/stub/cog" <<STUB
#!/usr/bin/env bash
case "\$STUB:\$1:\${2-}" in
  changelog-fails:changelog:*) exit 1 ;;
  bump-prints-junk:bump:*) echo "Next version: v9.9.9"; exit 0 ;;
  minor-prints-junk:bump:--minor) echo "Next version: v9.9.9"; exit 0 ;;
esac
exec "$real_cog" "\$@"
STUB
chmod +x "$tmp/stub/cog"
for stub_case in "changelog-fails:fix!" "bump-prints-junk:fix" "bump-prints-junk:fix!" "minor-prints-junk:fix!"; do
  stub=${stub_case%%:*}
  type=${stub_case#*:}
  repo
  commit -m "feat: x"
  git tag v0.1.0
  commit -m "$type: y"
  STUB=$stub PATH="$tmp/stub:$PATH" expect "cog $stub, after a $type" error
done

# The release notes (.github/release-notes.tera, set in cog.toml): a
# breaking change's mark, scope and footer, the footer's later lines
# indented under it.
repo
mkdir .github
cp "$root/.github/release-notes.tera" .github/
git add -A
git commit -q -m "chore: notes template"
commit -m "refactor(api)!: move the routes" -m $'BREAKING CHANGE: the routes moved:\n- /v1/x is now /x'
git tag v0.1.0
if ! notes=$(cog changelog --at v0.1.0 2>"$tmp/stderr"); then
  echo "FAIL release notes: cog changelog failed"
  sed 's/^/     /' "$tmp/stderr"
  fails=$((fails + 1))
else
  missing=0
  for line in '- **BREAKING:** **api:** move the routes ([' '  - the routes moved:' '    - /v1/x is now /x'; do
    if [[ $'\n'$notes != *$'\n'"$line"* ]]; then
      echo "FAIL release notes: no line starting \"$line\""
      missing=1
    fi
  done
  if ((missing)); then
    echo "     ${notes//$'\n'/$'\n'     }"
    fails=$((fails + 1))
  else
    echo "ok   release notes: breaking mark, scope and footer"
  fi
fi

if ((fails > 0)); then
  echo "$fails failed"
  exit 1
fi
