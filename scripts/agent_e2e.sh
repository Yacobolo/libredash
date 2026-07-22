#!/usr/bin/env bash
set -euo pipefail

ROOT="$(cd "$(dirname "${BASH_SOURCE[0]}")/.." && pwd)"
cd "$ROOT"

if [[ -f "$HOME/.zshrc.secrets" ]]; then
  # shellcheck disable=SC1090
  source "$HOME/.zshrc.secrets"
fi

DEEPSEEK_KEY="${DEEPSEEK_API_TOKEN:-${DEEPSEEK_API_KEY:-}}"
if [[ -z "$DEEPSEEK_KEY" ]]; then
  echo "DEEPSEEK_API_TOKEN or DEEPSEEK_API_KEY is required in ~/.zshrc.secrets" >&2
  exit 1
fi

go run ./internal/tools/bootstrapolist

TMP_DIR="$(mktemp -d)"
cleanup() {
  if [[ -n "${SERVER_PID:-}" ]]; then
    kill "$SERVER_PID" 2>/dev/null || true
    wait "$SERVER_PID" 2>/dev/null || true
  fi
  rm -rf "$TMP_DIR"
}
trap cleanup EXIT

BIN="$TMP_DIR/leapview"
go build -o "$BIN" ./cmd/leapview

PORT="$(python3 - <<'PY'
import socket
s = socket.socket()
s.bind(("127.0.0.1", 0))
print(s.getsockname()[1])
s.close()
PY
)"
TARGET="http://127.0.0.1:$PORT"

export LEAPVIEW_HOME="$TMP_DIR/home"
export LEAPVIEW_ADDR="127.0.0.1:$PORT"
export LEAPVIEW_PRODUCTION=true
export LEAPVIEW_PUBLIC_URL="https://localhost"
export LEAPVIEW_ALLOWED_HOSTS="127.0.0.1,localhost"
export LEAPVIEW_ENVIRONMENT=dev
export LEAPVIEW_API_TOKEN_ONLY_AUTH=true
export LEAPVIEW_CSRF_KEY="agent-e2e-csrf-key-agent-e2e-csrf-key"
export LEAPVIEW_METRICS_BEARER_TOKEN="agent-e2e-metrics-token-agent-e2e"
export LEAPVIEW_AGENT_API_KEY="$DEEPSEEK_KEY"
export LEAPVIEW_AGENT_BASE_URL="https://api.deepseek.com"
export LEAPVIEW_AGENT_MODEL="deepseek-v4-flash"

export LEAPVIEW_BOOTSTRAP_ADMIN_EMAIL=agent-e2e@example.com
INITIAL_CREDENTIALS="$("$BIN" admin initialize --format json)"
TOKEN="$(python3 -c 'import json,sys; print(json.load(sys.stdin)["publisherToken"])' <<<"$INITIAL_CREDENTIALS")"
"$BIN" serve > "$TMP_DIR/server.log" 2>&1 &
SERVER_PID="$!"

for _ in {1..80}; do
  if curl -fsS -H "Authorization: Bearer $TOKEN" "$TARGET/api/workspaces" >/dev/null 2>&1; then
    break
  fi
  if ! kill -0 "$SERVER_PID" 2>/dev/null; then
    cat "$TMP_DIR/server.log" >&2
    exit 1
  fi
  sleep 0.25
done

SYNC_OUTPUT="$("$BIN" data sync --project dashboards/leapview.yaml --connection olist --from .data/olist --target "$TARGET" --token "$TOKEN")"
echo "$SYNC_OUTPUT"
REVISION="$(awk '$1 == "staged" { print $2 }' <<<"$SYNC_OUTPUT")"
[[ "$REVISION" =~ ^sha256:[0-9a-f]{64}$ ]] || {
  echo "managed data sync did not return a canonical revision" >&2
  exit 1
}
"$BIN" deploy --target "$TARGET" --token "$TOKEN" --project dashboards/leapview.yaml --revision "olist=$REVISION" --auto-approve

OUTPUT="$("$BIN" agent ask "List the dashboards I can use in the sales workspace and mention the Olist context." --target "$TARGET" --token "$TOKEN" --json)"
echo "$OUTPUT"

if ! grep -Eiq 'executive|sales|olist' <<<"$OUTPUT"; then
  echo "agent response did not mention expected workspace facts" >&2
  exit 1
fi
