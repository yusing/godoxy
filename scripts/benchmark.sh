#!/bin/bash
# Benchmark script to compare GoDoxy, Traefik, Caddy, and Nginx
# Uses h2load for throughput benchmarks and a custom probe client from cmd/bench_server
# for real-world latency scenarios such as JSON APIs, uploads, streaming, SSE, and WebSocket.

set -e

# Configuration
HOST="bench.domain.com"
DURATION="${DURATION:-10s}"
THREADS="${THREADS:-4}"
CONNECTIONS="${CONNECTIONS:-100}"
REQUESTS="${REQUESTS:-1000}"
STREAMS="${STREAMS:-100}"
TARGET="${TARGET-}"
H1="${H1:-1}"
H2="${H2:-1}"
H3="${H3:-1}"
CONNECTION_MODE="${CONNECTION_MODE:-both}"
H3_TOOL="${H3_TOOL:-auto}"
LATENCY_SAMPLES="${LATENCY_SAMPLES:-5}"
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
  --h1 / --no-h1        Enable/disable HTTP/1.1 checks and benchmark (default: enabled)
  --h2 / --no-h2        Enable/disable HTTP/2 checks and benchmark (default: enabled)
  --h3 / --no-h3        Enable/disable HTTP/3 checks and benchmark (default: enabled)
  --reused              Run duration-based reused-connection benchmarks only
  --fresh               Run fixed-request fresh-connection benchmarks only
  --both                Run both reused and fresh modes (default)
  -h, --help            Show this help

Environment:
  H1=0|1                Enable/disable HTTP/1.1 (default: 1)
  H2=0|1                Enable/disable HTTP/2 (default: 1)
  H3=0|1                Enable/disable HTTP/3 (default: 1)
  CONNECTION_MODE=both|reused|fresh
  STREAMS=100           Concurrent streams per H2/H3 session in reused mode
  REQUESTS=1000         Requests per protocol in fresh mode
                         Fresh mode uses -c REQUESTS to force one request per connection.
  H3_TOOL=auto|h2load|h3bench
  LATENCY_SAMPLES=5     Samples per real-world latency scenario
  UPLOAD_BODY_BYTES=262144  Request body size for upload latency probe
  TARGET=<service>      Limit benchmark to GoDoxy, Traefik, Caddy, or Nginx
  DURATION=10s THREADS=4 CONNECTIONS=100
EOF2
}

for arg in "$@"; do
	case "$arg" in
	--h1) H1=1 ;;
	--no-h1) H1=0 ;;
	--h2) H2=1 ;;
	--no-h2) H2=0 ;;
	--h3) H3=1 ;;
	--no-h3) H3=0 ;;
	--reused) CONNECTION_MODE=reused ;;
	--fresh) CONNECTION_MODE=fresh ;;
	--both) CONNECTION_MODE=both ;;
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
done

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

case "${CONNECTION_MODE,,}" in
reused | reuse) CONNECTION_MODE=reused ;;
fresh | new) CONNECTION_MODE=fresh ;;
both | all) CONNECTION_MODE=both ;;
*)
	red "Error: CONNECTION_MODE must be reused, fresh, or both (got $CONNECTION_MODE)"
	exit 2
	;;
esac

if [ "$H1" = "0" ] && [ "$H2" = "0" ] && [ "$H3" = "0" ]; then
	red "Error: at least one protocol must be enabled"
	exit 2
fi

./scripts/ensure_benchmark_cert.sh

if { [ "$H1" = "1" ] || [ "$H2" = "1" ]; } && ! command -v h2load &>/dev/null; then
	red "Error: h2load is not installed (required for HTTP/1.1 and HTTP/2; use --no-h1/--no-h2 to skip)"
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
			build_h3bench
		fi
		;;
	h2load)
		if ! command -v h2load &>/dev/null || ! h2load --help 2>&1 | grep -q -- "--h3"; then
			yellow "h2load does not expose --h3; falling back to h3bench"
			H3_TOOL="h3bench"
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
echo "Duration: $DURATION"
echo "Threads: $THREADS"
echo "Connections: $CONNECTIONS"
echo "Requests: $REQUESTS"
echo "Streams: $STREAMS"
echo "Connection mode: $CONNECTION_MODE"
echo "Latency samples: $LATENCY_SAMPLES"
echo "Upload probe bytes: $UPLOAD_BODY_BYTES"
echo "HTTP/1.1: $H1"
echo "HTTP/2: $H2"
echo "HTTP/3: $H3"
if [ "$H3" = "1" ]; then
	echo "HTTP/3 tool: $H3_TOOL"
fi
if [ -n "$TARGET" ]; then
	echo "Filter: $TARGET"
fi
echo ""

# Define services to test
declare -A services=(
	["GoDoxy"]="http://127.0.0.1:8080"
	["Traefik"]="http://127.0.0.1:8081"
	["Caddy"]="http://127.0.0.1:8082"
	["Nginx"]="http://127.0.0.1:8083"
)

h3_port() {
	case "$1" in
	GoDoxy) echo "8440" ;;
	Traefik) echo "8441" ;;
	Caddy) echo "8442" ;;
	Nginx) echo "8443" ;;
	esac
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
	local attempt
	local res

	for attempt in {1..30}; do
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
	local attempt

	for attempt in {1..30}; do
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
	h3bench) "$H3BENCH_CMD" -d 1s -c 1 -dial "$dial_addr" -k "$url" >/dev/null 2>&1 ;;
	esac
}

enabled_protocols_label() {
	local protocols=()
	[ "$H1" = "1" ] && protocols+=("HTTP/1.1")
	[ "$H2" = "1" ] && protocols+=("HTTP/2")
	[ "$H3" = "1" ] && protocols+=("HTTP/3")
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

blue "========================================"
blue "Connection Tests"
blue "========================================"
echo ""

# Run connection tests for all services
for name in "${!services[@]}"; do
	if [ -z "$TARGET" ] || [ "${name,,}" = "${TARGET,,}" ]; then
		test_connection "$name" "${services[$name]}"
	fi
done

echo ""
blue "========================================"

# Exit if any connection test failed
if [ ${#connection_errors[@]} -gt 0 ]; then
	echo ""
	red "Connection test failed for the following services:"
	for error in "${connection_errors[@]}"; do
		red "  - $error"
	done
	echo ""
	red "Please ensure all services are running before benchmarking"
	exit 1
fi

echo ""
green "All services are reachable. Starting benchmarks..."
echo ""
blue "========================================"
echo ""

restart_bench() {
	local name=$1
	echo ""
	yellow "Restarting bench service before benchmarking $name..."
	docker compose -f dev.compose.yml up -d --force-recreate bench -t 0 >/dev/null 2>&1
	sleep 1
}

filter_h2load_noise() {
	grep -vE "^(starting benchmark...|spawning thread|progress: |Warm-up |Main benchmark duration|Stopped all clients|Process Request Failure)"
}

run_reused_benchmark() {
	local name=$1
	local url=$2
	local https_url=$3
	local tls_connect_arg=$4
	local h3_dial_addr=$5
	local h2_duration="${DURATION%s}"

	echo ""
	blue "[Reused connections] duration=$DURATION connections=$CONNECTIONS streams=$STREAMS"

	restart_bench "$name"
	if [ "$H1" = "1" ]; then
		echo ""
		echo "[HTTP/1.1 reused TLS] h2load --h1 -m1"
		h2load --h1 -t"$THREADS" -c"$CONNECTIONS" -m1 --duration="$h2_duration" \
			"$tls_connect_arg" \
			-H "Host: $HOST" \
			"$https_url" | filter_h2load_noise
	fi

	if [ "$H2" = "1" ]; then
		echo ""
		restart_bench "$name"
		echo "[HTTP/2 reused TLS] h2load -m$STREAMS"
		h2load -t"$THREADS" -c"$CONNECTIONS" -m"$STREAMS" --duration="$h2_duration" \
			"$tls_connect_arg" \
			-H ":authority: $HOST" \
			"$https_url" | filter_h2load_noise
	fi

	if [ "$H3" = "1" ]; then
		echo ""
		restart_bench "$name"
		echo "[HTTP/3 reused] $H3_TOOL"
		case "$H3_TOOL" in
			h2load)
				h2load -t"$THREADS" -c"$CONNECTIONS" -m"$STREAMS" --duration="$h2_duration" \
					--h3 \
					"$tls_connect_arg" \
					-H ":authority: $HOST" \
					"$https_url" | filter_h2load_noise
				;;
			h3bench)
				"$H3BENCH_CMD" -d "$DURATION" -c "$CONNECTIONS" -dial "$h3_dial_addr" -k "$https_url"
				;;
		esac
	fi
}

run_fresh_benchmark() {
	local name=$1
	local url=$2
	local https_url=$3
	local tls_connect_arg=$4
	echo ""
	blue "[Fresh connections] requests=$REQUESTS one-request-per-connection"

	if [ "$H1" = "1" ]; then
		restart_bench "$name"
		echo ""
		echo "[HTTP/1.1 fresh TLS] h2load --h1 -m1"
		h2load --h1 -t"$THREADS" -c"$REQUESTS" -n"$REQUESTS" -m1 \
			"$tls_connect_arg" \
			-H "Host: $HOST" \
			"$https_url" | filter_h2load_noise
	fi

	if [ "$H2" = "1" ]; then
		restart_bench "$name"
		echo ""
		echo "[HTTP/2 fresh TLS] h2load -m1"
		h2load -t"$THREADS" -c"$REQUESTS" -n"$REQUESTS" -m1 \
			"$tls_connect_arg" \
			-H ":authority: $HOST" \
			"$https_url" | filter_h2load_noise
	fi

	if [ "$H3" = "1" ]; then
		if [ "$H3_TOOL" != "h2load" ]; then
			yellow "Skipping HTTP/3 fresh benchmark: H3_TOOL=$H3_TOOL does not support fixed-request fresh mode"
			return
		fi
		restart_bench "$name"
		echo ""
		echo "[HTTP/3 fresh] h2load --h3 -m1"
		h2load -t"$THREADS" -c"$REQUESTS" -n"$REQUESTS" -m1 \
			--h3 \
			"$tls_connect_arg" \
			-H ":authority: $HOST" \
			"$https_url" | filter_h2load_noise
	fi
}

run_benchmark() {
	local name=$1
	local url=$2
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
	echo "Cleartext URL: $url (not used for protocol benchmarks)"
	echo "========================================"

	case "$CONNECTION_MODE" in
	reused) run_reused_benchmark "$name" "$url" "$https_url" "$tls_connect_arg" "$h3_dial_addr" ;;
	fresh) run_fresh_benchmark "$name" "$url" "$https_url" "$tls_connect_arg" ;;
	both)
		run_reused_benchmark "$name" "$url" "$https_url" "$tls_connect_arg" "$h3_dial_addr"
		run_fresh_benchmark "$name" "$url" "$https_url" "$tls_connect_arg"
		;;
	esac

	restart_bench "$name"
	run_real_world_probes "$name"

	echo ""
	green "✓ $name benchmark completed"
	blue "----------------------------------------"
	echo ""
}

for name in "${!services[@]}"; do
	if [ -z "$TARGET" ] || [ "${name,,}" = "${TARGET,,}" ]; then
		run_benchmark "$name" "${services[$name]}"
	fi
done

blue "========================================"
blue "Benchmark Summary"
blue "========================================"
echo ""
echo "All benchmark output saved to: $OUTFILE"
echo ""
echo "Enabled protocols: $(enabled_protocols_label)"
echo "Connection mode: $CONNECTION_MODE"
echo "Key metrics to compare:"
echo "  - Requests/sec (throughput)"
echo "  - Latency (mean, stdev)"
echo "  - Transfer/sec"
echo "  - Real-world latency probes: dial, TTFB, total, payload bytes"
echo "  - Scenarios: JSON API, upload, streaming body, SSE, WebSocket"
if [ "$H3" = "1" ]; then
	echo "  - HTTP/3 QUIC stats (RTT, packets sent/recv/lost)"
fi
echo ""
green "All benchmarks completed!"
