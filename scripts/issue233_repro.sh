#!/usr/bin/env bash

set -euo pipefail

COMPOSE_FILE="${COMPOSE_FILE:-issue233.compose.yml}"
PROJECT="${PROJECT:-godoxy-issue233-repro}"
HTTPS_URL="${HTTPS_URL:-http://127.0.0.1:18080/}"
API_URL="${API_URL:-http://127.0.0.1:18888/api/v1/version}"
DURATION="${DURATION:-60}"
OUT_DIR="${OUT_DIR:-profiles/issue233-repro}"

red() { printf '\033[0;31m%s\033[0m\n' "$*"; }
green() { printf '\033[0;32m%s\033[0m\n' "$*"; }
blue() { printf '\033[0;34m%s\033[0m\n' "$*"; }

mkdir -p "$OUT_DIR"
STAMP=$(date +%Y%m%d-%H%M%S)
RUN_DIR="$OUT_DIR/$STAMP"
mkdir -p "$RUN_DIR"

cleanup() {
  docker compose -f "$COMPOSE_FILE" -p "$PROJECT" logs --no-color >"$RUN_DIR/compose.logs.txt" 2>/dev/null || true
  docker compose -f "$COMPOSE_FILE" -p "$PROJECT" down -t 0 --remove-orphans >/dev/null 2>&1 || true
}
trap cleanup EXIT

wait_ready() {
  local elapsed=0
  until curl -fsS --max-time 2 "$API_URL" >/dev/null; do
    sleep 1
    elapsed=$((elapsed + 1))
    if [[ "$elapsed" -ge 120 ]]; then
      return 1
    fi
  done
}

collect_pprof() {
  curl -fsS "http://127.0.0.1:17777/debug/pprof/goroutine?debug=2" >"$RUN_DIR/goroutine.txt" || true
  curl -fsS "http://127.0.0.1:17777/debug/pprof/heap" >"$RUN_DIR/heap.pb.gz" || true
  curl -fsS "http://127.0.0.1:17777/debug/pprof/allocs" >"$RUN_DIR/allocs.pb.gz" || true
}

blue "build + start repro stack"
docker compose -f "$COMPOSE_FILE" -p "$PROJECT" up -d --build --force-recreate

blue "wait for API ready"
wait_ready

blue "start traffic + flapping backend for ${DURATION}s"
end_ts=$(( $(date +%s) + DURATION ))
req_ok=0
req_fail=0
flaps=0

while [[ "$(date +%s)" -lt "$end_ts" ]]; do
  if curl -fsS --max-time 2 -H 'Host: issue233.local' "$HTTPS_URL" >/dev/null; then
    req_ok=$((req_ok + 1))
  else
    req_fail=$((req_fail + 1))
  fi

  docker compose -f "$COMPOSE_FILE" -p "$PROJECT" stop -t 0 backend >/dev/null 2>&1 || true
  sleep 0.2
  docker compose -f "$COMPOSE_FILE" -p "$PROJECT" start backend >/dev/null 2>&1 || true
  flaps=$((flaps + 1))
done

collect_pprof

printf 'req_ok=%s\nreq_fail=%s\nflaps=%s\n' "$req_ok" "$req_fail" "$flaps" | tee "$RUN_DIR/summary.txt"

if [[ "$req_fail" -gt 0 ]]; then
  red "repro observed failures; artifacts: $RUN_DIR"
  exit 1
fi

green "no request failures observed; artifacts: $RUN_DIR"
