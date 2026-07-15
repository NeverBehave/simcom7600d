#!/usr/bin/env bash
# Starts the Go fixture server, exports URL + TOKEN, runs the JS test, kills server.
set -euo pipefail

ROOT="$(cd "$(dirname "$0")"/../../../.. && pwd)"
cd "$ROOT"

# Compile the fixture server binary first (respects the //go:build ignore tag
# only when used as a package; explicit file path bypasses it).
BIN_TMP=$(mktemp "${TMPDIR:-/tmp}/sim7600d_smoke_server.XXXXXX")
trap 'rm -f "$BIN_TMP"' EXIT
go build -o "$BIN_TMP" web/client/test/fixtures/server.go

# Start fixture server in background, capture URL/TOKEN from its stdout
TMP_OUT=$(mktemp)
trap 'kill "$SERVER_PID" 2>/dev/null || true; rm -f "$TMP_OUT" "$BIN_TMP"' EXIT

"$BIN_TMP" > "$TMP_OUT" &
SERVER_PID=$!

# Wait for the server to write URL=... and TOKEN=...
for _ in $(seq 1 50); do
  if grep -q '^URL=' "$TMP_OUT" && grep -q '^TOKEN=' "$TMP_OUT"; then
    break
  fi
  sleep 0.1
done

URL_LINE=$(grep '^URL=' "$TMP_OUT")
TOKEN_LINE=$(grep '^TOKEN=' "$TMP_OUT")
URL="${URL_LINE#URL=}"
TOKEN="${TOKEN_LINE#TOKEN=}"

if [ -z "$URL" ] || [ -z "$TOKEN" ]; then
  echo "fixture server didn't print URL/TOKEN within 5 seconds" >&2
  cat "$TMP_OUT" >&2
  exit 1
fi

cd "$ROOT/web/client"
SIM7600D_URL="$URL" SIM7600D_TOKEN="$TOKEN" \
  npx tsx --test test/sdk_smoke.test.ts
