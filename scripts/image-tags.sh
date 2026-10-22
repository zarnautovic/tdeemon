#!/usr/bin/env bash
# Prints the tags of a release's image, one a line: X.Y.Z, X.Y and
# sha-<the commit's first 12 characters>, on ghcr.io/<the repository in
# lower case>. The release workflow runs it.
#
#   scripts/image-tags.sh v1.2.3 <commit> <owner/repo>
set -euo pipefail

semver='^v([0-9]+\.[0-9]+)\.[0-9]+$'
# The version is matched last, so BASH_REMATCH holds its X.Y.
if [[ $# -ne 3 || ! $2 =~ ^[0-9a-f]{40}$ || ! $3 =~ ^[^/]+/[^/]+$ || ! $1 =~ $semver ]]; then
  echo "usage: $0 vX.Y.Z <40-character commit> <owner/repo>" >&2
  exit 2
fi
image=ghcr.io/${3,,}
echo "$image:${1#v}"
echo "$image:${BASH_REMATCH[1]}"
echo "$image:sha-${2::12}"
