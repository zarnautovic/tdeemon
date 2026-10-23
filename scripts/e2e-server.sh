#!/usr/bin/env bash
# Runs the server of the end-to-end tests (web/playwright.config.ts): the
# binary built with the web app, on 127.0.0.1:<port>, over a fresh data
# directory with one user, whose address and password are E2E_EMAIL and
# E2E_PASSWORD. It runs on macOS's bash 3.2 too.
#
#   E2E_EMAIL=… E2E_PASSWORD=… scripts/e2e-server.sh <binary> <port> <data dir>
set -euo pipefail

if [[ $# -ne 3 || ! $2 =~ ^[0-9]+$ || -z ${E2E_EMAIL:-} || -z ${E2E_PASSWORD:-} ]]; then
  echo "usage: E2E_EMAIL=… E2E_PASSWORD=… $0 <binary> <port> <data dir>" >&2
  exit 2
fi
bin=$1 port=$2 dir=$3

rm -rf "$dir"
mkdir -p "$dir"
export TDEEMON_DATA_DIR=$dir
export TDEEMON_BASE_URL=http://127.0.0.1:$port
export TDEEMON_LOG_FORMAT=text
export TDEEMON_BACKUP_KEEP=0
export TDEEMON_LOG_LEVEL=warn
TDEEMON_SECRET_KEY=$(head -c 32 /dev/urandom | base64)
export TDEEMON_SECRET_KEY

# A user needs the database, which the server creates when it starts; it
# starts first on another port, so the tests, which wait for the port,
# begin only once the user exists.
TDEEMON_LISTEN=127.0.0.1:$((port + 1)) "$bin" &
first=$!
for _ in $(seq 100); do
  if curl -fs -o /dev/null "http://127.0.0.1:$((port + 1))/healthz"; then
    break
  fi
  sleep 0.1
done
kill "$first"
wait "$first" || true

printf '%s\n' "$E2E_PASSWORD" |
  "$bin" user create --email "$E2E_EMAIL" --name Ana --timezone Europe/Zagreb --password-stdin >/dev/null

TDEEMON_LISTEN=127.0.0.1:$port exec "$bin"
