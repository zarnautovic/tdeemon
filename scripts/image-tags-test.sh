#!/usr/bin/env bash
# Tests scripts/image-tags.sh. Run by `make test-scripts`.
set -uo pipefail
root=$(cd "$(dirname "$0")/.." && pwd)
fails=0
sha=0123456789abcdef0123456789abcdef01234567

# expect NAME WANT ARGS...: the script prints WANT, or with WANT=fail
# exits with an error and prints nothing.
expect() {
  local name=$1 want=$2 got
  shift 2
  if got=$("$root/scripts/image-tags.sh" "$@" 2>/dev/null); then
    :
  else
    got="fail${got:+ after: $got}"
  fi
  if [[ $got == "$want" ]]; then
    echo "ok   $name"
  else
    printf 'FAIL %s: want\n%s\ngot\n%s\n' "$name" "$want" "$got"
    fails=$((fails + 1))
  fi
}

expect "a release" "ghcr.io/zarnautovic/tdeemon:1.2.3
ghcr.io/zarnautovic/tdeemon:1.2
ghcr.io/zarnautovic/tdeemon:sha-0123456789ab" v1.2.3 "$sha" zarnautovic/tdeemon
expect "two-digit parts" "ghcr.io/o/r:10.20.30
ghcr.io/o/r:10.20
ghcr.io/o/r:sha-0123456789ab" v10.20.30 "$sha" o/r
expect "the repository in lower case" "ghcr.io/zarnautovic/tdeemon:0.1.0
ghcr.io/zarnautovic/tdeemon:0.1
ghcr.io/zarnautovic/tdeemon:sha-0123456789ab" v0.1.0 "$sha" ZArnautovic/TDEEMON
expect "no v" fail 1.2.3 "$sha" o/r
expect "a pre-release" fail v1.2.3-rc.1 "$sha" o/r
expect "a short commit" fail v1.2.3 0123456 o/r
expect "a repository without an owner" fail v1.2.3 "$sha" tdeemon
expect "too few arguments" fail v1.2.3 "$sha"
expect "too many arguments" fail v1.2.3 "$sha" o/r extra
expect "a repository with a tag" fail v1.2.3 "$sha" o/r:latest

# macOS's /bin/bash is 3.2, which has no case-changing expansions.
if grep -nE '[$][{][^}]*(,,|\^\^)' "$root/scripts/image-tags.sh"; then
  echo "FAIL bash 3.2: a case-changing expansion above"
  fails=$((fails + 1))
else
  echo "ok   bash 3.2"
fi

if ((fails > 0)); then
  echo "$fails failed"
  exit 1
fi
