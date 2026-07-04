#!/usr/bin/env bash

set -euo pipefail

COMPOSE_FILE="${COMPOSE_FILE:-pprof.compose.yml}"
IMAGE_NAME="${IMAGE_NAME:-godoxy-pprof-repro}"
PROJECT_PREFIX="${PROJECT_PREFIX:-godoxy-pprof-repro}"
OUT_DIR="${OUT_DIR:-profiles/pprof-repro}"
WORKERS="${WORKERS:-1}"
TRIES="${TRIES:-0}"
READY_TIMEOUT="${READY_TIMEOUT:-45}"
STABILIZE_TIMEOUT="${STABILIZE_TIMEOUT:-5}"
CAPTURE_ON_SUCCESS="${CAPTURE_ON_SUCCESS:-1}"
PPROF_TOP_COUNT="${PPROF_TOP_COUNT:-30}"
POLL_INTERVAL="${POLL_INTERVAL:-1}"
PPROF_SECONDS="${PPROF_SECONDS:-20}"
API_PATH="${API_PATH:-/api/v1/version}"
API_HOST_PORT_BASE="${API_HOST_PORT_BASE:-18888}"
PPROF_HOST_PORT_BASE="${PPROF_HOST_PORT_BASE:-17777}"
DEBUG_HOST_PORT_BASE="${DEBUG_HOST_PORT_BASE:-17778}"
HTTP_HOST_PORT_BASE="${HTTP_HOST_PORT_BASE:-18080}"
HTTPS_HOST_PORT_BASE="${HTTPS_HOST_PORT_BASE:-18443}"

red() { printf '\033[0;31m%s\033[0m\n' "$*"; }
green() { printf '\033[0;32m%s\033[0m\n' "$*"; }
yellow() { printf '\033[1;33m%s\033[0m\n' "$*"; }
blue() { printf '\033[0;34m%s\033[0m\n' "$*"; }

mkdir -p "$OUT_DIR"

build_image() {
  blue "build pprof image: $IMAGE_NAME"
  docker build \
    --build-arg SHADOWTREE_ARGS=mode=pprof \
    --target=main \
    -t "$IMAGE_NAME" .
}

port_for() {
  local base=$1
  local worker=$2
  printf '%s' $((base + (worker - 1) * 10))
}

curl_ok() {
  local url=$1
  curl -fsS --max-time 2 "$url" >/dev/null
}

wait_for_ready() {
  local api_url=$1
  local timeout=$2
  local elapsed=0

  while [[ "$elapsed" -lt "$timeout" ]]; do
    if curl_ok "$api_url"; then
      return 0
    fi
    sleep "$POLL_INTERVAL"
    elapsed=$((elapsed + POLL_INTERVAL))
  done

  return 1
}

collect_pprof() {
  local base_url=$1
  local run_dir=$2
  local container_id=${3:-}

  blue "collect pprof: $base_url"
  curl -fsS "$base_url/debug/pprof/goroutine?debug=2" >"$run_dir/goroutine.txt" || true
  curl -fsS "$base_url/debug/pprof/heap" >"$run_dir/heap.pb.gz" || true
  curl -fsS "$base_url/debug/pprof/profile?seconds=$PPROF_SECONDS" >"$run_dir/cpu.pb.gz" || true
  curl -fsS "$base_url/debug/pprof/allocs" >"$run_dir/allocs.pb.gz" || true

  if [[ -n "$container_id" ]]; then
    docker cp "$container_id:/app/run" "$run_dir/run.bin" >/dev/null 2>&1 || true
    if [[ -s "$run_dir/run.bin" && -s "$run_dir/cpu.pb.gz" ]]; then
      go tool pprof -top -nodecount="$PPROF_TOP_COUNT" "$run_dir/run.bin" "$run_dir/cpu.pb.gz" >"$run_dir/cpu.top.txt" 2>&1 || true
    fi
  fi
}

worker_loop() {
  local worker=$1
  local tries=0
  local project="${PROJECT_PREFIX}-${worker}"
  local api_port
  local pprof_port
  local debug_port
  local http_port
  local https_port
  local api_url
  local pprof_url

  api_port=$(port_for "$API_HOST_PORT_BASE" "$worker")
  pprof_port=$(port_for "$PPROF_HOST_PORT_BASE" "$worker")
  debug_port=$(port_for "$DEBUG_HOST_PORT_BASE" "$worker")
  http_port=$(port_for "$HTTP_HOST_PORT_BASE" "$worker")
  https_port=$(port_for "$HTTPS_HOST_PORT_BASE" "$worker")

  api_url="http://127.0.0.1:${api_port}${API_PATH}"
  pprof_url="http://127.0.0.1:${pprof_port}"

  while :; do
    if [[ "$TRIES" -gt 0 && "$tries" -ge "$TRIES" ]]; then
      green "worker $worker done after $tries tries"
      return 0
    fi
    tries=$((tries + 1))

    local stamp run_dir container_id
    stamp=$(date +%Y%m%d-%H%M%S)
    run_dir="$OUT_DIR/worker-${worker}/try-${tries}-${stamp}"
    mkdir -p "$run_dir"

    blue "worker $worker try $tries: up"
    API_HOST_PORT="$api_port" \
      PPROF_HOST_PORT="$pprof_port" \
      DEBUG_HOST_PORT="$debug_port" \
      HTTP_HOST_PORT="$http_port" \
      HTTPS_HOST_PORT="$https_port" \
      docker compose -f "$COMPOSE_FILE" -p "$project" up -d --force-recreate --build >/dev/null

    container_id=$(docker compose -f "$COMPOSE_FILE" -p "$project" ps -q app)
    if [[ -z "$container_id" ]]; then
      red "worker $worker try $tries: no container id"
      docker compose -f "$COMPOSE_FILE" -p "$project" logs --no-color >"$run_dir/logs.txt" || true
      docker compose -f "$COMPOSE_FILE" -p "$project" down -t 0 --remove-orphans >/dev/null 2>&1 || true
      continue
    fi

    if wait_for_ready "$api_url" "$READY_TIMEOUT"; then
      green "worker $worker try $tries: api ready"
      sleep "$STABILIZE_TIMEOUT"
      docker compose -f "$COMPOSE_FILE" -p "$project" logs --no-color >"$run_dir/logs.txt" || true
      if [[ "$CAPTURE_ON_SUCCESS" == "1" ]]; then
        collect_pprof "$pprof_url" "$run_dir" "$container_id"
      fi
      docker compose -f "$COMPOSE_FILE" -p "$project" down -t 0 --remove-orphans >/dev/null 2>&1 || true
      continue
    fi

    yellow "worker $worker try $tries: api not ready after $READY_TIMEOUT s"
    docker compose -f "$COMPOSE_FILE" -p "$project" logs --no-color >"$run_dir/logs.txt" || true
    collect_pprof "$pprof_url" "$run_dir" "$container_id"
    docker compose -f "$COMPOSE_FILE" -p "$project" down -t 0 --remove-orphans >/dev/null 2>&1 || true
    red "repro found: $run_dir"
    return 1
  done
}

cleanup() {
  local status=$?
  if [[ "${KEEP:-0}" == "1" ]]; then
    return "$status"
  fi

  blue "cleanup compose projects"
  local worker
  for worker in $(seq 1 "$WORKERS"); do
    docker compose -f "$COMPOSE_FILE" -p "${PROJECT_PREFIX}-${worker}" down -t 0 --remove-orphans >/dev/null 2>&1 || true
  done
  return "$status"
}
trap cleanup EXIT

build_image

if [[ "$WORKERS" -le 1 ]]; then
  worker_loop 1
  exit $?
fi

worker_pids=""
worker=1
while [[ "$worker" -le "$WORKERS" ]]; do
  worker_loop "$worker" &
  worker_pids="$worker_pids $!"
  worker=$((worker + 1))
done

status=0
for pid in $worker_pids; do
  wait "$pid" || status=$?
  if [[ "$status" -ne 0 ]]; then
    break
  fi
done

if [[ "$status" -ne 0 ]]; then
  red "repro found in parallel run"
  exit "$status"
fi

green "all workers finished"
