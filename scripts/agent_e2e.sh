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

go run ./internal/tools/bootstrapolist --out .data/olist

TMP_DIR="$(mktemp -d)"
cleanup() {
  if [[ -n "${SERVER_PID:-}" ]]; then
    kill "$SERVER_PID" 2>/dev/null || true
    wait "$SERVER_PID" 2>/dev/null || true
  fi
  chmod -R u+w "$TMP_DIR" 2>/dev/null || true
  rm -rf "$TMP_DIR"
}
trap cleanup EXIT

BIN="$TMP_DIR/leapview"
go build -tags=duckdb_arrow -o "$BIN" ./cmd/leapview

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
export LEAPVIEW_PRODUCTION=false
export LEAPVIEW_ENVIRONMENT=dev
export LEAPVIEW_API_TOKEN_ONLY_AUTH=false
export LEAPVIEW_LOCAL_AUTH=false
export LEAPVIEW_DEV_API_TOKEN="agent-e2e-dev-token"
export LEAPVIEW_CSRF_KEY="agent-e2e-csrf-key-agent-e2e-csrf-key"
export LEAPVIEW_METRICS_BEARER_TOKEN="agent-e2e-metrics-token-agent-e2e"
export LEAPVIEW_AGENT_API_KEY="$DEEPSEEK_KEY"
export LEAPVIEW_AGENT_BASE_URL="https://api.deepseek.com"
export LEAPVIEW_AGENT_MODEL="deepseek-v4-flash"

TOKEN="$LEAPVIEW_DEV_API_TOKEN"
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

ALLOWED_TOOLS='catalog_search,catalog_list,catalog_get,query_semantic_model,query_dashboard_visual,query_visual,docs_search,docs_read'

run_agent_scenario() {
  local label="$1"
  local expected_tools="$2"
  local question="$3"
  local output conversation stop_reason messages

  output="$("$BIN" agent ask "$question" --target "$TARGET" --token "$TOKEN" --json)"
  echo "$label: $output"
  conversation="$(python3 -c 'import json,sys; print(json.load(sys.stdin)["conversationId"])' <<<"$output")"
  stop_reason="$(python3 -c 'import json,sys; print(json.load(sys.stdin)["run"]["stopReason"])' <<<"$output")"
  if [[ "$stop_reason" != "completed" ]]; then
    echo "$label: agent run stopped with $stop_reason" >&2
    exit 1
  fi
  messages="$(curl -fsS -H "Authorization: Bearer $TOKEN" "$TARGET/api/v1/agent/conversations/$conversation/messages?limit=200")"
  python3 -c '
import json
import sys

label, expected_csv, allowed_csv = sys.argv[1:4]
payload = json.load(sys.stdin)
names = [item.get("toolName", "") for item in payload.get("items", []) if item.get("toolName")]
allowed = set(allowed_csv.split(","))
legacy = sorted(set(names) - allowed)
if legacy:
    raise SystemExit(f"{label}: transcript used non-curated tools: {legacy}")
expected = [name for name in expected_csv.split(",") if name]
missing = [name for name in expected if name not in names]
if missing:
    raise SystemExit(f"{label}: transcript missed expected tools {missing}; used {names}")
successful = {
    item.get("toolName", "")
    for item in payload.get("items", [])
    if item.get("toolName") and not item.get("isError", False)
}
failed = [name for name in expected if name not in successful]
if failed:
    raise SystemExit(f"{label}: expected tools never succeeded {failed}; used {names}")
' "$label" "$expected_tools" "$ALLOWED_TOOLS" <<<"$messages"
}

run_agent_scenario \
  "workspace discovery" \
  "catalog_list" \
  "Use catalog browsing to list the workspaces I can access."

run_agent_scenario \
  "global dashboard search" \
  "catalog_search" \
  "Use global catalog search to find the executive sales dashboard across all workspaces."

run_agent_scenario \
  "dashboard and page browsing" \
  "catalog_search,catalog_list,catalog_get" \
  "Find the executive sales dashboard with catalog search, browse its pages with catalog list, then inspect the overview page definition with catalog get."

run_agent_scenario \
  "semantic query" \
  "catalog_search,query_semantic_model" \
  "Find the semantic model named Sales in the sales workspace, then query the order_count measure with the governed semantic query tool. Use the exact catalog reference IDs returned."

run_agent_scenario \
  "dashboard visual query" \
  "catalog_search,query_dashboard_visual" \
  "Find the revenue_kpi visual on the Executive Sales overview page in the sales workspace, then query that exact dashboard visual using its returned ref and location."

run_agent_scenario \
  "generated visualization" \
  "catalog_search,query_visual" \
  "Find the orders table, orders.status field, and order_count measure in the Sales semantic model, then create a read-only bar visualization using their exact catalog reference IDs."

run_agent_scenario \
  "documentation" \
  "docs_search,docs_read" \
  "Search the LeapView product documentation for semantic relationships, read the relevant document, and summarize the documented behavior."
