#!/usr/bin/env bash
set -euo pipefail

ROOT="$(cd "$(dirname "${BASH_SOURCE[0]}")/.." && pwd)"
PROFILE_ROOT="/Users/yacobolo/.config/leapview/profiles"

usage() {
  echo "Usage: $0 quack start|stop|status|logs" >&2
}

profile="${1:-}"
action="${2:-}"

if [[ -z "$profile" || -z "$action" ]]; then
  usage
  exit 2
fi

case "$profile" in
  quack) ;;
  *)
    echo "Unsupported LeapView dev profile: $profile" >&2
    usage
    exit 2
    ;;
esac

case "$action" in
  start|stop|status|logs) ;;
  *)
    echo "Unsupported LeapView dev profile action: $action" >&2
    usage
    exit 2
    ;;
esac

profile_dir="$PROFILE_ROOT/$profile"
env_file="$profile_dir/env"
secrets_file="$profile_dir/secrets.env"

if [[ ! -f "$env_file" ]]; then
  echo "Missing LeapView dev profile env: $env_file" >&2
  exit 1
fi

existing_quack_token="${LEAPVIEW_QUACK_TOKEN:-}"
existing_agent_api_key="${LEAPVIEW_AGENT_API_KEY:-}"
existing_home="${LEAPVIEW_HOME:-}"
existing_duckdb_dir="${LEAPVIEW_DUCKDB_DIR:-}"

set -a
# shellcheck source=/dev/null
source "$env_file"
if [[ -f "$secrets_file" ]]; then
  # shellcheck source=/dev/null
  source "$secrets_file"
fi
set +a

if [[ -z "${LEAPVIEW_QUACK_TOKEN:-}" && -n "$existing_quack_token" ]]; then
  export LEAPVIEW_QUACK_TOKEN="$existing_quack_token"
fi
if [[ -z "${LEAPVIEW_AGENT_API_KEY:-}" && -n "$existing_agent_api_key" ]]; then
  export LEAPVIEW_AGENT_API_KEY="$existing_agent_api_key"
fi
export LEAPVIEW_HOME="${existing_home:-$ROOT/.tmp/profiles/$profile/home}"
export LEAPVIEW_DUCKDB_DIR="${existing_duckdb_dir:-$ROOT/.tmp/profiles/$profile/duckdb}"

for dir in "${LEAPVIEW_HOME:-}" "${LEAPVIEW_DUCKDB_DIR:-}"; do
  if [[ -n "$dir" ]]; then
    mkdir -p "$dir"
  fi
done

pid_cwd() {
  local pid="$1"
  if command -v lsof >/dev/null 2>&1; then
    lsof -a -p "$pid" -d cwd -Fn 2>/dev/null | sed -n 's/^n//p' | head -n 1
  elif [[ -e "/proc/$pid/cwd" ]]; then
    readlink "/proc/$pid/cwd" 2>/dev/null || true
  fi
}

stop_pid() {
  local pid="$1"
  local label="${2:-process}"
  if ! kill -0 "$pid" 2>/dev/null; then
    return 0
  fi

  echo "Stopping $label (pid $pid)"
  kill "$pid" 2>/dev/null || true
  for _ in {1..30}; do
    if ! kill -0 "$pid" 2>/dev/null; then
      return 0
    fi
    sleep 0.1
  done
  echo "Force stopping $label (pid $pid)"
  kill -KILL "$pid" 2>/dev/null || true
}

stop_profile_duckdb_locks() {
  if [[ -z "${LEAPVIEW_DUCKDB_DIR:-}" ]]; then
    return 0
  fi
  if ! command -v lsof >/dev/null 2>&1; then
    return 0
  fi

  local db_file="$LEAPVIEW_DUCKDB_DIR/leapview-$profile.duckdb"
  [[ -e "$db_file" ]] || return 0

  local pids
  pids="$(lsof -t "$db_file" 2>/dev/null | sort -u || true)"
  [[ -n "$pids" ]] || return 0

  while read -r pid; do
    [[ -n "$pid" ]] || continue
    if [[ "$(pid_cwd "$pid")" == "$ROOT" ]]; then
      stop_pid "$pid" "stale LeapView $profile DuckDB lock holder"
    fi
  done <<< "$pids"

  local remaining
  remaining="$(lsof -t "$db_file" 2>/dev/null | sort -u || true)"
  if [[ -n "$remaining" ]]; then
    echo "LeapView profile '$profile' DuckDB is still locked by another process: $remaining" >&2
    echo "DuckDB file: $db_file" >&2
    exit 1
  fi
}

if [[ "$profile" == "quack" && "$action" == "start" ]]; then
  if [[ -z "${LEAPVIEW_QUACK_TOKEN:-}" ]]; then
    echo "Missing LEAPVIEW_QUACK_TOKEN. Add it to $secrets_file before running task dev:quack." >&2
    exit 1
  fi
  if [[ -z "${LEAPVIEW_AGENT_API_KEY:-}" ]]; then
    echo "Missing LEAPVIEW_AGENT_API_KEY. Add it to $secrets_file before running task dev:quack." >&2
    exit 1
  fi
  if grep -q "replace-with-quack-host" "$profile_dir/model.yaml"; then
    echo "Missing real Quack host. Update $profile_dir/model.yaml before running task dev:quack." >&2
    exit 1
  fi
fi

case "$action" in
  start)
    stop_profile_duckdb_locks
    exec "$ROOT/scripts/dev-server.sh" "$action"
    ;;
  stop)
    "$ROOT/scripts/dev-server.sh" "$action"
    stop_profile_duckdb_locks
    ;;
  status|logs)
    exec "$ROOT/scripts/dev-server.sh" "$action"
    ;;
esac
