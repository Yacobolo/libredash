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

BIN="$TMP_DIR/libredash"
go build -o "$BIN" ./cmd/libredash

PORT="$(python3 - <<'PY'
import socket
s = socket.socket()
s.bind(("127.0.0.1", 0))
print(s.getsockname()[1])
s.close()
PY
)"
TARGET="http://127.0.0.1:$PORT"

export LIBREDASH_HOME="$TMP_DIR/home"
export LIBREDASH_ADDR="127.0.0.1:$PORT"
export LIBREDASH_PRODUCTION=true
export LIBREDASH_API_TOKEN_ONLY_AUTH=true
export LIBREDASH_CSRF_KEY="agent-e2e-csrf-key-agent-e2e-csrf-key"
export LIBREDASH_METRICS_BEARER_TOKEN="agent-e2e-metrics-token-agent-e2e"
export LIBREDASH_AGENT_API_KEY="$DEEPSEEK_KEY"
export LIBREDASH_AGENT_BASE_URL="https://api.deepseek.com"
export LIBREDASH_AGENT_MODEL="deepseek-v4-flash"

TOKEN="$("$BIN" admin bootstrap --workspace sales)"
"$BIN" serve --workspace sales > "$TMP_DIR/server.log" 2>&1 &
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

"$BIN" publish --target "$TARGET" --token "$TOKEN" --workspace sales --project dashboards/libredash.yaml --auto-approve

OUTPUT="$("$BIN" agent ask "List the dashboards I can use in this workspace and mention the Olist context." --target "$TARGET" --token "$TOKEN" --workspace sales --json)"
echo "$OUTPUT"

if ! grep -Eiq 'executive|sales|olist' <<<"$OUTPUT"; then
  echo "agent response did not mention expected workspace facts" >&2
  exit 1
fi
