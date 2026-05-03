#!/bin/bash
# Benchmark script to compare GoDoxy, Traefik, Caddy, and Nginx
# Uses h2load for throughput benchmarks and a custom probe client from cmd/bench_server
# for real-world latency scenarios such as JSON APIs, uploads, streaming, SSE, and WebSocket.

set -euo pipefail

# Configuration
HOST="${HOST:-bench.domain.com}"
BENCH_PROFILE="${BENCH_PROFILE:-smoke}"
TARGET="${TARGET-}"
H1="${H1:-1}"
H2="${H2:-1}"
H3="${H3:-1}"
H1C="${H1C:-0}"
CONNECTION_MODE="${CONNECTION_MODE:-both}"
H3_TOOL="${H3_TOOL:-auto}"
BENCH_COMPOSE_FILE="${BENCH_COMPOSE_FILE:-dev.compose.yml}"
BENCH_COMPOSE_MANAGE="${BENCH_COMPOSE_MANAGE:-1}"
BENCH_COMPOSE_CLEANUP="${BENCH_COMPOSE_CLEANUP:-1}"
BENCH_COMPOSE_RECREATE="${BENCH_COMPOSE_RECREATE:-1}"
UPLOAD_BODY_BYTES="${UPLOAD_BODY_BYTES:-262144}"
PROBE_BUILD_CMD="${PROBE_BUILD_CMD:-go build -C cmd/bench_server -o $PWD/bin/bench_probe .}"
PROBE_CMD="${PROBE_CMD:-$PWD/bin/bench_probe}"

# Color functions for output
red() { echo -e "\033[0;31m$*\033[0m"; }
green() { echo -e "\033[0;32m$*\033[0m"; }
yellow() { echo -e "\033[1;33m$*\033[0m"; }
blue() { echo -e "\033[0;34m$*\033[0m"; }

usage() {
	cat <<EOF2
Usage: $0 [options]

Options:
  --profile NAME        Benchmark profile: smoke, stable, or stress (default: BENCH_PROFILE or smoke)
  --h1 / --no-h1        Enable/disable HTTP/1.1 checks and benchmark (default: enabled)
  --h2 / --no-h2        Enable/disable HTTP/2 checks and benchmark (default: enabled)
  --h3 / --no-h3        Enable/disable HTTP/3 checks and benchmark (default: enabled)
  --h1c / --no-h1c      Enable/disable cleartext HTTP/1.1 baseline benchmark (default: disabled)
  --reused              Run duration-based reused-connection benchmarks only
  --fresh               Run fixed-request fresh-connection benchmarks only
  --both                Run both reused and fresh modes (default)
  --smoke               Alias for --profile smoke
  --stable              Alias for --profile stable
  --stress              Alias for --profile stress
  -h, --help            Show this help

Environment:
  BENCH_PROFILE=smoke|stable|stress
                        smoke: quick broad correctness, lower concurrency
                        stable: repeated real-ish comparison, median/CV summary
                        stress: high-concurrency overload/limit testing
  H1=0|1                Enable/disable HTTP/1.1 (default: 1)
  H2=0|1                Enable/disable HTTP/2 (default: 1)
  H3=0|1                Enable/disable HTTP/3 (default: 1)
  H1C=0|1               Enable/disable cleartext HTTP/1.1 baseline on 8080..8083 (default: 0)
  CONNECTION_MODE=both|reused|fresh
  STREAMS=<N>           Concurrent streams per H2/H3 session (profile default)
  REQUESTS=<N>          Requests per protocol in fresh mode (profile default)
  FRESH_CONNECTIONS=<N> Concurrent connections in fresh mode (default: CONNECTIONS)
  H2LOAD_DURATION=<N>   h2load duration value without unit (derived from DURATION=Ns by default)
  H2LOAD_WARM_UP_TIME=<DURATION>  h2load warm-up before reused duration measurements (profile default; 0 disables)
  H3_TOOL=auto|h2load|h3bench
  BENCH_COMPOSE_MANAGE=0|1   Start selected proxy services and bench (default: 1)
  BENCH_COMPOSE_CLEANUP=0|1  Stop selected proxy services and bench on exit (default: 1)
  BENCH_COMPOSE_RECREATE=0|1 Force recreate compose services when starting/restarting (default: 1)
  BENCH_COMPOSE_FILE=dev.compose.yml
  LATENCY_SAMPLES=<N>   Samples per real-world latency scenario (profile default)
  RUNS=<N>              Repeat each throughput benchmark and print median/CV summary
  REPEAT_DELAY=1        Delay between repeated throughput runs, in seconds
  UPLOAD_BODY_BYTES=262144  Request body size for upload latency probe
  TARGET=<service>      Limit benchmark to GoDoxy, Traefik, Caddy, or Nginx
  DURATION=<D> THREADS=<N> CONNECTIONS=<N>

Profiles:
  smoke   DURATION=10s THREADS=4 CONNECTIONS=32 STREAMS=16 REQUESTS=2000 FRESH_CONNECTIONS=32 RUNS=1 LATENCY_SAMPLES=5
  stable  DURATION=30s THREADS=4 CONNECTIONS=64 STREAMS=16 REQUESTS=20000 FRESH_CONNECTIONS=64 RUNS=5 LATENCY_SAMPLES=25
  stress  DURATION=30s THREADS=4 CONNECTIONS=100 STREAMS=100 REQUESTS=50000 FRESH_CONNECTIONS=100 RUNS=3 LATENCY_SAMPLES=10
EOF2
}

apply_bench_profile() {
	BENCH_PROFILE="${BENCH_PROFILE,,}"
	case "$BENCH_PROFILE" in
	smoke)
		DURATION="${DURATION:-10s}"
		H2LOAD_DURATION="${H2LOAD_DURATION:-}"
		H2LOAD_WARM_UP_TIME="${H2LOAD_WARM_UP_TIME:-3s}"
		THREADS="${THREADS:-4}"
		CONNECTIONS="${CONNECTIONS:-32}"
		REQUESTS="${REQUESTS:-2000}"
		FRESH_CONNECTIONS="${FRESH_CONNECTIONS:-${CONNECTIONS}}"
		STREAMS="${STREAMS:-16}"
		LATENCY_SAMPLES="${LATENCY_SAMPLES:-5}"
		RUNS="${RUNS:-1}"
		REPEAT_DELAY="${REPEAT_DELAY:-1}"
		BENCH_STARTUP_WAIT="${BENCH_STARTUP_WAIT:-1}"
		;;
	stable)
		DURATION="${DURATION:-30s}"
		H2LOAD_DURATION="${H2LOAD_DURATION:-}"
		H2LOAD_WARM_UP_TIME="${H2LOAD_WARM_UP_TIME:-5s}"
		THREADS="${THREADS:-4}"
		CONNECTIONS="${CONNECTIONS:-64}"
		REQUESTS="${REQUESTS:-20000}"
		FRESH_CONNECTIONS="${FRESH_CONNECTIONS:-${CONNECTIONS}}"
		STREAMS="${STREAMS:-16}"
		LATENCY_SAMPLES="${LATENCY_SAMPLES:-25}"
		RUNS="${RUNS:-5}"
		REPEAT_DELAY="${REPEAT_DELAY:-1}"
		BENCH_STARTUP_WAIT="${BENCH_STARTUP_WAIT:-3}"
		;;
	stress)
		DURATION="${DURATION:-30s}"
		H2LOAD_DURATION="${H2LOAD_DURATION:-}"
		H2LOAD_WARM_UP_TIME="${H2LOAD_WARM_UP_TIME:-5s}"
		THREADS="${THREADS:-4}"
		CONNECTIONS="${CONNECTIONS:-100}"
		REQUESTS="${REQUESTS:-50000}"
		FRESH_CONNECTIONS="${FRESH_CONNECTIONS:-${CONNECTIONS}}"
		STREAMS="${STREAMS:-100}"
		LATENCY_SAMPLES="${LATENCY_SAMPLES:-10}"
		RUNS="${RUNS:-3}"
		REPEAT_DELAY="${REPEAT_DELAY:-1}"
		BENCH_STARTUP_WAIT="${BENCH_STARTUP_WAIT:-3}"
		;;
	*)
		red "Error: BENCH_PROFILE must be smoke, stable, or stress (got $BENCH_PROFILE)"
		exit 2
		;;
	esac
}

while [ "$#" -gt 0 ]; do
	arg=$1
	case "$arg" in
	--profile)
		if [ "$#" -lt 2 ]; then
			red "Error: --profile requires smoke, stable, or stress"
			exit 2
		fi
		BENCH_PROFILE=$2
		shift
		;;
	--profile=*) BENCH_PROFILE=${arg#*=} ;;
	--h1) H1=1 ;;
	--no-h1) H1=0 ;;
	--h2) H2=1 ;;
	--no-h2) H2=0 ;;
	--h3) H3=1 ;;
	--no-h3) H3=0 ;;
	--h1c) H1C=1 ;;
	--no-h1c) H1C=0 ;;
	--reused) CONNECTION_MODE=reused ;;
	--fresh) CONNECTION_MODE=fresh ;;
	--both) CONNECTION_MODE=both ;;
	--stable) BENCH_PROFILE=stable ;;
	--stress) BENCH_PROFILE=stress ;;
	--smoke) BENCH_PROFILE=smoke ;;
	-h | --help)
		usage
		exit 0
		;;
	*)
		red "Error: unsupported option $arg"
		usage
		exit 2
		;;
	esac
	shift
done

apply_bench_profile

normalize_protocol_flag() {
	local name=$1
	local value=$2
	case "${value,,}" in
	1 | true | yes | on | enabled) echo 1 ;;
	0 | false | no | off | disabled) echo 0 ;;
	*)
		red "Error: $name must be on/off, true/false, or 1/0 (got $value)" >&2
		exit 2
		;;
	esac
}

H1=$(normalize_protocol_flag H1 "$H1")
H2=$(normalize_protocol_flag H2 "$H2")
H3=$(normalize_protocol_flag H3 "$H3")
H1C=$(normalize_protocol_flag H1C "$H1C")
BENCH_COMPOSE_MANAGE=$(normalize_protocol_flag BENCH_COMPOSE_MANAGE "$BENCH_COMPOSE_MANAGE")
BENCH_COMPOSE_CLEANUP=$(normalize_protocol_flag BENCH_COMPOSE_CLEANUP "$BENCH_COMPOSE_CLEANUP")
BENCH_COMPOSE_RECREATE=$(normalize_protocol_flag BENCH_COMPOSE_RECREATE "$BENCH_COMPOSE_RECREATE")

normalize_h2load_duration_arg() {
	local name=$1
	local value=$2

	case "$value" in
	0)
		return
		;;
	'' | *[!0-9hms]*)
		red "Error: $name must be a h2load duration like 3s, 500ms, 1m, or 0 to disable (got $value)"
		exit 2
		;;
	esac

	if ! [[ "$value" =~ ^[0-9]+(ms|s|m|h)?$ ]]; then
		red "Error: $name must be a h2load duration like 3s, 500ms, 1m, or 0 to disable (got $value)"
		exit 2
	fi

	if [[ "$value" =~ ^0+(ms|s|m|h)?$ ]]; then
		red "Error: $name must be positive, or exactly 0 to disable (got $value)"
		exit 2
	fi
}

normalize_duration() {
	if [ -n "$H2LOAD_DURATION" ]; then
		case "$H2LOAD_DURATION" in
		*[!0-9]*)
			red "Error: H2LOAD_DURATION must be an integer number of seconds (got $H2LOAD_DURATION)"
			exit 2
			;;
		esac
		return
	fi

	case "$DURATION" in
	*[!0-9]s)
		red "Error: DURATION must be an integer number of seconds like 10s (got $DURATION)"
		exit 2
		;;
	*s) H2LOAD_DURATION="${DURATION%s}" ;;
	*[!0-9]*)
		red "Error: DURATION must be an integer number of seconds like 10s (got $DURATION)"
		exit 2
		;;
	*)
		H2LOAD_DURATION="$DURATION"
		DURATION="${DURATION}s"
		;;
	esac
}

normalize_positive_int() {
	local name=$1
	local value=$2
	case "$value" in
	'' | *[!0-9]*)
		red "Error: $name must be a positive integer (got $value)"
		exit 2
		;;
	esac
	if [ "$value" -le 0 ]; then
		red "Error: $name must be greater than zero (got $value)"
		exit 2
	fi
}

normalize_duration
normalize_h2load_duration_arg H2LOAD_WARM_UP_TIME "$H2LOAD_WARM_UP_TIME"
H2LOAD_WARM_UP_ARGS=()
if [ "$H2LOAD_WARM_UP_TIME" != "0" ]; then
	H2LOAD_WARM_UP_ARGS=(--warm-up-time="$H2LOAD_WARM_UP_TIME")
fi
normalize_positive_int THREADS "$THREADS"
normalize_positive_int CONNECTIONS "$CONNECTIONS"
normalize_positive_int REQUESTS "$REQUESTS"
normalize_positive_int FRESH_CONNECTIONS "$FRESH_CONNECTIONS"
normalize_positive_int STREAMS "$STREAMS"
normalize_positive_int LATENCY_SAMPLES "$LATENCY_SAMPLES"
normalize_positive_int RUNS "$RUNS"
normalize_positive_int REPEAT_DELAY "$REPEAT_DELAY"
normalize_positive_int BENCH_STARTUP_WAIT "$BENCH_STARTUP_WAIT"

if [ "$BENCH_COMPOSE_MANAGE" = "0" ]; then
	BENCH_COMPOSE_CLEANUP=0
fi

case "${CONNECTION_MODE,,}" in
reused | reuse) CONNECTION_MODE=reused ;;
fresh | new) CONNECTION_MODE=fresh ;;
both | all) CONNECTION_MODE=both ;;
*)
	red "Error: CONNECTION_MODE must be reused, fresh, or both (got $CONNECTION_MODE)"
	exit 2
	;;
esac

if [ "$H1" = "0" ] && [ "$H2" = "0" ] && [ "$H3" = "0" ] && [ "$H1C" = "0" ]; then
	red "Error: at least one protocol must be enabled"
	exit 2
fi

./scripts/ensure_benchmark_cert.sh

if { [ "$H1" = "1" ] || [ "$H2" = "1" ] || [ "$H1C" = "1" ]; } && ! command -v h2load &>/dev/null; then
	red "Error: h2load is not installed (required for HTTP/1.1, cleartext HTTP/1.1, and HTTP/2; use --no-h1/--no-h1c/--no-h2 to skip)"
	echo "Please install nghttp2-client:"
	echo "  Ubuntu/Debian: sudo apt-get install nghttp2-client"
	echo "  macOS: brew install nghttp2"
	exit 1
fi

build_h3bench() {
	H3BENCH_CMD="${H3BENCH_CMD:-$PWD/bin/h3bench}"
	if [ ! -x "$H3BENCH_CMD" ] || [ cmd/h3bench/main.go -nt "$H3BENCH_CMD" ]; then
		yellow "Building local h3bench..."
		go build -C cmd/h3bench -o "$H3BENCH_CMD" .
	fi
}

build_probe_client() {
	if [ ! -x "$PROBE_CMD" ] || [ cmd/bench_server/main.go -nt "$PROBE_CMD" ] || [ cmd/bench_server/handler.go -nt "$PROBE_CMD" ] || [ cmd/bench_server/probe.go -nt "$PROBE_CMD" ]; then
		yellow "Building local bench probe client..."
		eval "$PROBE_BUILD_CMD"
	fi
}

H3BENCH_CMD=""
if [ "$H3" = "1" ]; then
	case "$H3_TOOL" in
	auto)
		if command -v h2load &>/dev/null && h2load --help 2>&1 | grep -q -- "--h3"; then
			H3_TOOL="h2load"
		else
			if command -v h2load &>/dev/null; then
				yellow "h2load does not expose --h3; falling back to h3bench"
			else
				yellow "h2load is not installed; falling back to h3bench"
			fi
			H3_TOOL="h3bench"
			yellow "HTTP/3 throughput results from h3bench are a fallback and are not directly comparable to h2load H1/H2 results"
			build_h3bench
		fi
		;;
	h2load)
		if ! command -v h2load &>/dev/null || ! h2load --help 2>&1 | grep -q -- "--h3"; then
			yellow "h2load does not expose --h3; falling back to h3bench"
			H3_TOOL="h3bench"
			yellow "HTTP/3 throughput results from h3bench are a fallback and are not directly comparable to h2load H1/H2 results"
			build_h3bench
		fi
		;;
	h3bench)
		build_h3bench
		;;
	*)
		red "Error: unsupported H3_TOOL=$H3_TOOL (use auto, h2load, or h3bench)"
		exit 1
		;;
	esac
fi

build_probe_client

OUTFILE="/tmp/reverse_proxy_benchmark_$(date +%Y%m%d_%H%M%S).log"
: >"$OUTFILE"
exec > >(tee -a "$OUTFILE") 2>&1

blue "========================================"
blue "Reverse Proxy Benchmark Comparison"
blue "========================================"
echo ""
echo "Target: $HOST"
echo "Profile: $BENCH_PROFILE"
echo "Duration: $DURATION"
echo "Threads: $THREADS"
echo "Connections: $CONNECTIONS"
echo "Requests: $REQUESTS"
echo "Streams: $STREAMS"
echo "Fresh connections: $FRESH_CONNECTIONS"
echo "Connection mode: $CONNECTION_MODE"
echo "Compose file: $BENCH_COMPOSE_FILE"
echo "Compose manage: $BENCH_COMPOSE_MANAGE"
echo "Compose cleanup: $BENCH_COMPOSE_CLEANUP"
echo "h2load duration seconds: $H2LOAD_DURATION"
echo "h2load warm-up time: $H2LOAD_WARM_UP_TIME"
echo "Latency samples: $LATENCY_SAMPLES"
echo "Throughput runs: $RUNS"
echo "Repeat delay: ${REPEAT_DELAY}s"
echo "Upload probe bytes: $UPLOAD_BODY_BYTES"
echo "HTTP/1.1: $H1"
echo "HTTP/2: $H2"
echo "HTTP/3: $H3"
echo "HTTP/1.1 cleartext baseline: $H1C"
if [ "$H3" = "1" ]; then
	echo "HTTP/3 tool: $H3_TOOL"
fi
if [ -n "$TARGET" ]; then
	echo "Filter: $TARGET"
fi
echo ""

# Define services to test
service_names=(GoDoxy Traefik Caddy Nginx)
declare -A services=(
	["GoDoxy"]="http://127.0.0.1:8080"
	["Traefik"]="http://127.0.0.1:8081"
	["Caddy"]="http://127.0.0.1:8082"
	["Nginx"]="http://127.0.0.1:8083"
)
declare -A compose_services=(
	["GoDoxy"]="godoxy"
	["Traefik"]="traefik"
	["Caddy"]="caddy"
	["Nginx"]="nginx"
)

service_selected() {
	local name=$1
	[ -z "$TARGET" ] || [ "${name,,}" = "${TARGET,,}" ]
}

matched_services=()
for name in "${service_names[@]}"; do
	if service_selected "$name"; then
		matched_services+=("$name")
	fi
done

if [ ${#matched_services[@]} -eq 0 ]; then
	red "Error: TARGET=$TARGET matched no services (valid: ${service_names[*]})"
	exit 2
fi

http_port() {
	case "$1" in
	GoDoxy) echo "8080" ;;
	Traefik) echo "8081" ;;
	Caddy) echo "8082" ;;
	Nginx) echo "8083" ;;
	esac
}

h3_port() {
	case "$1" in
	GoDoxy) echo "8440" ;;
	Traefik) echo "8441" ;;
	Caddy) echo "8442" ;;
	Nginx) echo "8443" ;;
	esac
}

http_url() {
	echo "http://$HOST:$(http_port "$1")/"
}

h2load_cleartext_connect_arg() {
	echo "--connect-to=127.0.0.1:$(http_port "$1")"
}

https_url() {
	echo "https://$HOST:$(h3_port "$1")/"
}

h3_url() {
	https_url "$1"
}

h3_dial_addr() {
	echo "127.0.0.1:$(h3_port "$1")"
}

h2load_tls_connect_arg() {
	echo "--connect-to=127.0.0.1:$(h3_port "$1")"
}

curl_resolve_arg() {
	echo "$HOST:$(h3_port "$1"):127.0.0.1"
}

read_response_with_retry() {
	local url=$1
	shift
	local curl_args=("$@")
	local res

	for _ in {1..30}; do
		if res=$(curl -sS -w "\n%{http_code}" "${curl_args[@]}" -H "Host: $HOST" --max-time 5 "$url" 2>/dev/null); then
			echo "$res"
			return 0
		fi
		sleep 1
	done

	curl -sS -w "\n%{http_code}" "${curl_args[@]}" -H "Host: $HOST" --max-time 5 "$url"
}

h3_check_with_retry() {
	local name=$1
	local url=$2
	local dial_addr
	dial_addr=$(h3_dial_addr "$name")
	local tls_connect_arg
	tls_connect_arg=$(h2load_tls_connect_arg "$name")
	for _ in {1..30}; do
		case "$H3_TOOL" in
		h2load)
			if h2load -n1 -c1 -m1 --h3 "$tls_connect_arg" -H ":authority: $HOST" "$url" >/dev/null 2>&1; then
				return 0
			fi
			;;
		h3bench)
			if "$H3BENCH_CMD" -d 1s -c 1 -dial "$dial_addr" -k "$url" >/dev/null 2>&1; then
				return 0
			fi
			;;
		esac
		sleep 1
	done

	case "$H3_TOOL" in
	h2load) h2load -n1 -c1 -m1 --h3 "$tls_connect_arg" -H ":authority: $HOST" "$url" >/dev/null 2>&1 ;;
	h3bench) "$H3BENCH_CMD" -d 1s -c 1 -m 1 -dial "$dial_addr" -k "$url" >/dev/null 2>&1 ;;
	esac
}

enabled_protocols_label() {
	local protocols=()
	[ "$H1" = "1" ] && protocols+=("HTTP/1.1")
	[ "$H2" = "1" ] && protocols+=("HTTP/2")
	[ "$H3" = "1" ] && protocols+=("HTTP/3")
	[ "$H1C" = "1" ] && protocols+=("HTTP/1.1 cleartext")
	local IFS=", "
	echo "${protocols[*]}"
}

proto_enabled() {
	case "$1" in
	h1) [ "$H1" = "1" ] ;;
	h2) [ "$H2" = "1" ] ;;
	h3) [ "$H3" = "1" ] ;;
	*) return 1 ;;
	esac
}

proto_label() {
	case "$1" in
	h1) echo "HTTP/1.1" ;;
	h2) echo "HTTP/2" ;;
	h3) echo "HTTP/3" ;;
	esac
}

probe_base_url() {
	echo "https://$HOST"
}

probe_ws_url() {
	echo "wss://$HOST/ws"
}

run_probe() {
	local kind=$1
	local proto=$2
	local url=$3
	local name=$4
	shift 4
	local args=("$@")
	local dial_addr
	dial_addr=$(h3_dial_addr "$name")

	if ! proto_enabled "$proto"; then
		return 0
	fi

	echo ""
	echo "[$(proto_label "$proto") $kind] $url"
	"$PROBE_CMD" \
		-probe "$kind" \
		-proto "$proto" \
		-url "$url" \
		-dial-addr "$dial_addr" \
		-samples "$LATENCY_SAMPLES" \
		-timeout 15s \
		"${args[@]}"
}

run_real_world_probes() {
	local name=$1
	local base_url
	base_url=$(probe_base_url "$name")

	echo ""
	blue "[Real-world latency probes] samples=$LATENCY_SAMPLES"

	for proto in h1 h2 h3; do
		run_probe http "$proto" "$base_url/json" "$name"
		run_probe http "$proto" "$base_url/upload" "$name" -method POST -body-bytes "$UPLOAD_BODY_BYTES"
		run_probe http "$proto" "$base_url/stream?chunks=8&chunk_bytes=4096&interval_ms=15" "$name"
		run_probe sse "$proto" "$base_url/sse?count=3&interval_ms=150" "$name"
	done

	# WebSocket upgrade is still the most common deployment shape and maps cleanly to gorilla/websocket.
	run_probe ws h1 "$(probe_ws_url)" "$name"
}

# Array to store connection errors
declare -a connection_errors=()
declare -a throughput_summaries=()

# Function to test connection before benchmarking
test_connection() {
	local name=$1
	local url=$2

	yellow "Testing connection to $name..."

	local https_url
	https_url=$(https_url "$name")
	local curl_resolve
	curl_resolve=$(curl_resolve_arg "$name")

	local failed=false
	if [ "$H1C" = "1" ]; then
		local clear_url
		clear_url=$(http_url "$name")
		local res0
		res0=$(read_response_with_retry "$clear_url" --http1.1)
		local body0
		body0=$(echo "$res0" | head -n -1)
		local status0
		status0=$(echo "$res0" | tail -n 1)

		if [ "$status0" != "200" ] || [ ${#body0} -ne 4096 ]; then
			red "✗ $name failed cleartext HTTP/1.1 connection test (Status: $status0, Body length: ${#body0})"
			failed=true
		fi
	fi

	if [ "$H1" = "1" ]; then
		local res1
		res1=$(read_response_with_retry "$https_url" --http1.1 --insecure --resolve "$curl_resolve")
		local body1
		body1=$(echo "$res1" | head -n -1)
		local status1
		status1=$(echo "$res1" | tail -n 1)

		if [ "$status1" != "200" ] || [ ${#body1} -ne 4096 ]; then
			red "✗ $name failed HTTP/1.1 connection test (Status: $status1, Body length: ${#body1})"
			failed=true
		fi
	fi

	if [ "$H2" = "1" ]; then
		local res2
		res2=$(read_response_with_retry "$https_url" --http2 --insecure --resolve "$curl_resolve")
		local body2
		body2=$(echo "$res2" | head -n -1)
		local status2
		status2=$(echo "$res2" | tail -n 1)

		if [ "$status2" != "200" ] || [ ${#body2} -ne 4096 ]; then
			red "✗ $name failed HTTP/2 connection test (Status: $status2, Body length: ${#body2})"
			failed=true
		fi
	fi

	if [ "$H3" = "1" ] && [ -n "$https_url" ]; then
		if ! h3_check_with_retry "$name" "$https_url"; then
			red "✗ $name failed HTTP/3 connection test (URL: $https_url)"
			failed=true
		fi
	fi

	if [ "$failed" = true ]; then
		connection_errors+=("$name failed connection test (URL: $url)")
		return 1
	else
		green "✓ $name is reachable ($(enabled_protocols_label))"
		return 0
	fi
}

compose_target_services() {
	local targets=(bench)
	local name
	for name in "${matched_services[@]}"; do
		targets+=("${compose_services[$name]}")
	done
	echo "${targets[@]}"
}

compose_up() {
	[ "$BENCH_COMPOSE_MANAGE" = "1" ] || return 0
	local targets
	targets=$(compose_target_services)
	yellow "Starting benchmark compose services: $targets"
	local recreate_args=()
	if [ "$BENCH_COMPOSE_RECREATE" = "1" ]; then
		recreate_args+=(--force-recreate)
	fi
	# shellcheck disable=SC2086 # targets are an intentional word list.
	docker compose -f "$BENCH_COMPOSE_FILE" up -d -t 0 "${recreate_args[@]}" $targets
	sleep "$BENCH_STARTUP_WAIT"
}

compose_down() {
	[ "$BENCH_COMPOSE_MANAGE" = "1" ] || return 0
	[ "$BENCH_COMPOSE_CLEANUP" = "1" ] || return 0
	local targets
	targets=$(compose_target_services)
	yellow "Stopping benchmark compose services: $targets"
	# shellcheck disable=SC2086 # targets are an intentional word list.
	docker compose -f "$BENCH_COMPOSE_FILE" down $targets -t 0
}

compose_up
trap compose_down EXIT

echo ""
green "Compose services ready. Starting benchmarks..."
echo ""
blue "========================================"
echo ""

restart_bench() {
	local name=$1
	echo ""
	yellow "Restarting benchmark services before benchmarking $name..."
	local recreate_args=()
	if [ "$BENCH_COMPOSE_RECREATE" = "1" ]; then
		recreate_args+=(--force-recreate)
	fi
	local targets=(bench)
	if [ -n "${compose_services[$name]:-}" ]; then
		targets+=("${compose_services[$name]}")
	fi
	docker compose -f "$BENCH_COMPOSE_FILE" up -d -t 0 "${recreate_args[@]}" "${targets[@]}" >/dev/null 2>&1
	sleep "$BENCH_STARTUP_WAIT"

	connection_errors=()
	if ! test_connection "$name" "${services[$name]}"; then
		echo ""
		red "Connection test failed for $name:"
		for error in "${connection_errors[@]}"; do
			red "  - $error"
		done
		echo ""
		red "Please ensure benchmark services are running, or leave BENCH_COMPOSE_MANAGE=1 so this script can start them"
		exit 1
	fi
}

wait_for_reused_benchmark_ready() {
	local name=$1
	local proto=$2
	case "$proto" in
	h1)
		read_response_with_retry "$(https_url "$name")" --insecure --http1.1 --resolve "$(curl_resolve_arg "$name")" >/dev/null
		;;
	h2)
		read_response_with_retry "$(https_url "$name")" --insecure --http2 --resolve "$(curl_resolve_arg "$name")" >/dev/null
		;;
	h3)
		h3_check_with_retry "$name" "$(https_url "$name")" >/dev/null
		;;
	esac
}

filter_h2load_noise() {
	grep -vE "(^|[[:space:]])([0-9]+\.)?[[:space:]]*Stopping all clients|Stopped all clients for thread|^[0-9]+$|^(starting benchmark...|spawning thread|progress: |Warm-up |Main benchmark duration)" || true
}

run_h2load() {
	BENCH_THROUGHPUT=""
	BENCH_FAILED=""
	local h2load_status
	local output_file
	output_file=$(mktemp)

	set +e
	h2load "$@" 2>&1 | filter_h2load_noise | tee "$output_file"
	h2load_status=${PIPESTATUS[0]}
	set -e

	BENCH_THROUGHPUT=$(awk '
		/finished in/ {
			for (i = 1; i <= NF; i++) {
				if ($i == "req/s," || $i == "req/s") {
					v = $(i - 1)
					gsub(",", "", v)
					print v
				}
			}
		}
	' "$output_file" | tail -n 1)
	BENCH_FAILED=$(awk '
		/^requests:/ {
			for (i = 1; i <= NF; i++) {
				if ($i == "failed,") {
					print $(i - 1)
				}
			}
		}
	' "$output_file" | tail -n 1)
	rm -f "$output_file"

	if [ "$h2load_status" -ne 0 ]; then
		yellow "h2load exited with status $h2load_status; continuing so non-2xx/stream failures remain visible in benchmark output"
	fi
}

run_h3bench() {
	BENCH_THROUGHPUT=""
	BENCH_FAILED=""
	local h3bench_status
	local output_file
	output_file=$(mktemp)

	set +e
	"$H3BENCH_CMD" "$@" 2>&1 | tee "$output_file"
	h3bench_status=${PIPESTATUS[0]}
	set -e

	BENCH_THROUGHPUT=$(awk '/^Throughput:/ { print $2 }' "$output_file" | tail -n 1)
	BENCH_FAILED=$(awk '/^Failed:/ { print $2 }' "$output_file" | tail -n 1)
	rm -f "$output_file"

	if [ "$h3bench_status" -ne 0 ]; then
		yellow "h3bench exited with status $h3bench_status; continuing so failures remain visible in benchmark output"
	fi
}

summarize_throughput() {
	local label=$1
	shift
	local values=("$@")

	if [ ${#values[@]} -eq 0 ]; then
		yellow "No parseable throughput results for $label"
		return
	fi

	local stats
	stats=$(printf '%s\n' "${values[@]}" | sort -n | awk '
		{
			a[++n] = $1
			sum += $1
			sumsq += $1 * $1
		}
		END {
			if (n == 0) {
				exit 1
			}
			mean = sum / n
			variance = (sumsq / n) - (mean * mean)
			if (variance < 0) {
				variance = 0
			}
			sd = sqrt(variance)
			if (n % 2 == 1) {
				median = a[(n + 1) / 2]
			} else {
				median = (a[n / 2] + a[n / 2 + 1]) / 2
			}
			cv = mean == 0 ? 0 : (sd / mean) * 100
			printf "runs=%d median=%.2f req/s mean=%.2f req/s sd=%.2f cv=%.2f%% min=%.2f max=%.2f", n, median, mean, sd, cv, a[1], a[n]
		}
	')

	echo ""
	green "[$label summary] $stats"
	throughput_summaries+=("$label | $stats")
}

run_throughput_series() {
	local label=$1
	shift
	local values=()
	local run

	for ((run = 1; run <= RUNS; run++)); do
		if [ "$RUNS" -gt 1 ]; then
			echo ""
			echo "[$label run $run/$RUNS]"
		fi

		"$@"

		if [ -n "${BENCH_FAILED:-}" ] && [ "$BENCH_FAILED" != "0" ]; then
			yellow "$label run $run reported $BENCH_FAILED failed requests; excluding this run from summary"
		elif [ -n "${BENCH_THROUGHPUT:-}" ]; then
			values+=("$BENCH_THROUGHPUT")
		else
			yellow "Could not parse throughput for $label run $run"
		fi

		if [ "$run" -lt "$RUNS" ]; then
			sleep "$REPEAT_DELAY"
		fi
	done

	if [ "$RUNS" -gt 1 ]; then
		summarize_throughput "$label" "${values[@]}"
	fi
}

run_reused_benchmark() {
	local name=$1
	local http_url=$2
	local cleartext_connect_arg=$3
	local https_url=$4
	local tls_connect_arg=$5
	local h3_dial_addr=$6

	echo ""
	blue "[Reused connections] duration=$DURATION warm-up=$H2LOAD_WARM_UP_TIME connections=$CONNECTIONS streams=$STREAMS"

	if [ "$H1C" = "1" ]; then
		restart_bench "$name"
		read_response_with_retry "$(http_url "$name")" --http1.1 >/dev/null
		echo ""
		echo "[HTTP/1.1 cleartext reused] h2load --h1 -m1"
		run_throughput_series "$name HTTP/1.1 cleartext reused" run_h2load \
			--h1 -t"$THREADS" -c"$CONNECTIONS" -m1 --duration="$H2LOAD_DURATION" \
			"${H2LOAD_WARM_UP_ARGS[@]}" \
			"$cleartext_connect_arg" \
			-H "Host: $HOST" \
			"$http_url"
	fi

	if [ "$H1" = "1" ]; then
		restart_bench "$name"
		wait_for_reused_benchmark_ready "$name" h1
		echo ""
		echo "[HTTP/1.1 reused TLS] h2load --h1 -m1"
		run_throughput_series "$name HTTP/1.1 reused TLS" run_h2load \
			--h1 -t"$THREADS" -c"$CONNECTIONS" -m1 --duration="$H2LOAD_DURATION" \
			"${H2LOAD_WARM_UP_ARGS[@]}" \
			"$tls_connect_arg" \
			-H "Host: $HOST" \
			"$https_url"
	fi

	if [ "$H2" = "1" ]; then
		echo ""
		restart_bench "$name"
		wait_for_reused_benchmark_ready "$name" h2
		echo "[HTTP/2 reused TLS] h2load -m$STREAMS"
		run_throughput_series "$name HTTP/2 reused TLS" run_h2load \
			-t"$THREADS" -c"$CONNECTIONS" -m"$STREAMS" --duration="$H2LOAD_DURATION" \
			"${H2LOAD_WARM_UP_ARGS[@]}" \
			"$tls_connect_arg" \
			-H ":authority: $HOST" \
			"$https_url"
	fi

	if [ "$H3" = "1" ]; then
		echo ""
		restart_bench "$name"
		wait_for_reused_benchmark_ready "$name" h3
		echo "[HTTP/3 reused] $H3_TOOL"
		case "$H3_TOOL" in
		h2load)
			run_throughput_series "$name HTTP/3 reused" run_h2load \
				-t"$THREADS" -c"$CONNECTIONS" -m"$STREAMS" --duration="$H2LOAD_DURATION" \
				"${H2LOAD_WARM_UP_ARGS[@]}" \
				--h3 \
				"$tls_connect_arg" \
				-H ":authority: $HOST" \
				"$https_url"
			;;
		h3bench)
			run_throughput_series "$name HTTP/3 reused" run_h3bench \
				-d "$DURATION" -c "$CONNECTIONS" -m "$STREAMS" -dial "$h3_dial_addr" -k "$https_url"
			;;
		esac
	fi
}

run_fresh_benchmark() {
	local name=$1
	local http_url=$2
	local cleartext_connect_arg=$3
	local https_url=$4
	local tls_connect_arg=$5
	local h3_dial_addr=$6
	echo ""
	blue "[Fresh connections] requests=$REQUESTS concurrency=$FRESH_CONNECTIONS one-request-per-connection"

	if [ "$H1C" = "1" ]; then
		restart_bench "$name"
		echo ""
		echo "[HTTP/1.1 cleartext fresh] h2load --h1 -m1"
		run_throughput_series "$name HTTP/1.1 cleartext fresh" run_h2load \
			--h1 -t"$THREADS" -c"$FRESH_CONNECTIONS" -n"$REQUESTS" -m1 \
			"$cleartext_connect_arg" \
			-H "Host: $HOST" \
			"$http_url"
	fi

	if [ "$H1" = "1" ]; then
		restart_bench "$name"
		echo ""
		echo "[HTTP/1.1 fresh TLS] h2load --h1 -m1"
		run_throughput_series "$name HTTP/1.1 fresh TLS" run_h2load \
			--h1 -t"$THREADS" -c"$FRESH_CONNECTIONS" -n"$REQUESTS" -m1 \
			"$tls_connect_arg" \
			-H "Host: $HOST" \
			"$https_url"
	fi

	if [ "$H2" = "1" ]; then
		restart_bench "$name"
		echo ""
		echo "[HTTP/2 fresh TLS] h2load -m1"
		run_throughput_series "$name HTTP/2 fresh TLS" run_h2load \
			-t"$THREADS" -c"$FRESH_CONNECTIONS" -n"$REQUESTS" -m1 \
			"$tls_connect_arg" \
			-H ":authority: $HOST" \
			"$https_url"
	fi

	if [ "$H3" = "1" ]; then
		restart_bench "$name"
		echo ""
		echo "[HTTP/3 fresh] $H3_TOOL -m1"
		case "$H3_TOOL" in
		h2load)
			run_throughput_series "$name HTTP/3 fresh" run_h2load \
				-t"$THREADS" -c"$FRESH_CONNECTIONS" -n"$REQUESTS" -m1 \
				--h3 \
				"$tls_connect_arg" \
				-H ":authority: $HOST" \
				"$https_url"
			;;
		h3bench)
			run_throughput_series "$name HTTP/3 fresh" run_h3bench \
				-n "$REQUESTS" -c "$FRESH_CONNECTIONS" -m 1 -dial "$h3_dial_addr" -k "$https_url"
			;;
		esac
	fi
}

run_benchmark() {
	local name=$1
	local http_url
	http_url=$(http_url "$name")
	local cleartext_connect_arg
	cleartext_connect_arg=$(h2load_cleartext_connect_arg "$name")
	local https_url
	https_url=$(https_url "$name")
	local tls_connect_arg
	tls_connect_arg=$(h2load_tls_connect_arg "$name")
	local h3_dial_addr
	h3_dial_addr=$(h3_dial_addr "$name")

	yellow "Testing $name..."

	echo "========================================"
	echo "$name"
	echo "Benchmark URL: $https_url ($tls_connect_arg)"
	echo "Cleartext URL: $http_url ($cleartext_connect_arg)"
	echo "========================================"

	case "$CONNECTION_MODE" in
	reused) run_reused_benchmark "$name" "$http_url" "$cleartext_connect_arg" "$https_url" "$tls_connect_arg" "$h3_dial_addr" ;;
	fresh) run_fresh_benchmark "$name" "$http_url" "$cleartext_connect_arg" "$https_url" "$tls_connect_arg" "$h3_dial_addr" ;;
	both)
		run_reused_benchmark "$name" "$http_url" "$cleartext_connect_arg" "$https_url" "$tls_connect_arg" "$h3_dial_addr"
		run_fresh_benchmark "$name" "$http_url" "$cleartext_connect_arg" "$https_url" "$tls_connect_arg" "$h3_dial_addr"
		;;
	esac

	restart_bench "$name"
	run_real_world_probes "$name"

	echo ""
	green "✓ $name benchmark completed"
	blue "----------------------------------------"
	echo ""
}

for name in "${matched_services[@]}"; do
	run_benchmark "$name"
done

blue "========================================"
blue "Benchmark Summary"
blue "========================================"
echo ""
echo "All benchmark output saved to: $OUTFILE"
echo ""
echo "Enabled protocols: $(enabled_protocols_label)"
echo "Connection mode: $CONNECTION_MODE"
if [ ${#throughput_summaries[@]} -gt 0 ]; then
	echo ""
	echo "Throughput repeat summaries:"
	for summary in "${throughput_summaries[@]}"; do
		echo "  - $summary"
	done
fi
echo "Key metrics to compare:"
echo "  - Requests/sec (throughput)"
if [ "$H1C" = "1" ]; then
	echo "  - Cleartext HTTP/1.1 baseline excludes TLS/ALPN overhead"
fi
echo "  - Latency (mean, stdev)"
echo "  - Transfer/sec"
echo "  - Real-world latency probes: dial, TTFB, total, payload bytes"
echo "  - Scenarios: JSON API, upload, streaming body, SSE, WebSocket"
if [ "$H3" = "1" ]; then
	echo "  - HTTP/3 QUIC stats (RTT, packets sent/recv/lost)"
fi
echo ""
green "All benchmarks completed!"
