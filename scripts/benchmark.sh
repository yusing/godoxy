#!/bin/bash
# Benchmark script to compare GoDoxy, Traefik, Caddy, and Nginx
# Uses wrk for HTTP/1.1, h2load for HTTP/2, and h2load/h3bench for HTTP/3 load testing

set -e

# Configuration
HOST="bench.domain.com"
DURATION="${DURATION:-10s}"
THREADS="${THREADS:-4}"
CONNECTIONS="${CONNECTIONS:-100}"
TARGET="${TARGET-}"
H1="${H1:-1}"
H2="${H2:-1}"
H3="${H3:-1}"
H3_TOOL="${H3_TOOL:-auto}"

# Color functions for output
red() { echo -e "\033[0;31m$*\033[0m"; }
green() { echo -e "\033[0;32m$*\033[0m"; }
yellow() { echo -e "\033[1;33m$*\033[0m"; }
blue() { echo -e "\033[0;34m$*\033[0m"; }

usage() {
	cat <<EOF
Usage: $0 [options]

Options:
  --h1 / --no-h1        Enable/disable HTTP/1.1 checks and benchmark (default: enabled)
  --h2 / --no-h2        Enable/disable HTTP/2 checks and benchmark (default: enabled)
  --h3 / --no-h3        Enable/disable HTTP/3 checks and benchmark (default: enabled)
  -h, --help            Show this help

Environment:
  H1=0|1                Enable/disable HTTP/1.1 (default: 1)
  H2=0|1                Enable/disable HTTP/2 (default: 1)
  H3=0|1                Enable/disable HTTP/3 (default: 1)
  H3_TOOL=auto|h2load|h3bench
  TARGET=<service>      Limit benchmark to GoDoxy, Traefik, Caddy, or Nginx
  DURATION=10s THREADS=4 CONNECTIONS=100
EOF
}

for arg in "$@"; do
	case "$arg" in
	--h1) H1=1 ;;
	--no-h1) H1=0 ;;
	--h2) H2=1 ;;
	--no-h2) H2=0 ;;
	--h3) H3=1 ;;
	--no-h3) H3=0 ;;
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

if [ "$H1" = "0" ] && [ "$H2" = "0" ] && [ "$H3" = "0" ]; then
	red "Error: at least one protocol must be enabled"
	exit 2
fi

./scripts/ensure_benchmark_cert.sh

if [ "$H1" = "1" ] && ! command -v wrk &>/dev/null; then
	red "Error: wrk is not installed (required for HTTP/1.1; use --no-h1 to skip)"
	echo "Please install wrk:"
	echo "  Ubuntu/Debian: sudo apt-get install wrk"
	echo "  macOS: brew install wrk"
	echo "  Or build from source: https://github.com/wg/wrk"
	exit 1
fi

if [ "$H2" = "1" ] && ! command -v h2load &>/dev/null; then
	red "Error: h2load is not installed (required for HTTP/2; use --no-h2 to skip)"
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

h3_url() {
	echo "https://$HOST:$(h3_port "$1")/"
}

h3_dial_addr() {
	echo "127.0.0.1:$(h3_port "$1")"
}


read_response_with_retry() {
	local curl_proto=$1
	local url=$2
	local attempt
	local res

	for attempt in {1..30}; do
		if res=$(curl -sS -w "\n%{http_code}" "$curl_proto" -H "Host: $HOST" --max-time 5 "$url" 2>/dev/null); then
			echo "$res"
			return 0
		fi
		sleep 1
	done

	curl -sS -w "\n%{http_code}" "$curl_proto" -H "Host: $HOST" --max-time 5 "$url"
}

h3_check_with_retry() {
	local name=$1
	local url=$2
	local dial_addr
	dial_addr=$(h3_dial_addr "$name")
	local attempt

	for attempt in {1..30}; do
		case "$H3_TOOL" in
		h2load)
			if h2load -k -n1 -c1 -m1 --h3 -H ":authority: $HOST" "$url" >/dev/null 2>&1; then
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
	h2load) h2load -k -n1 -c1 -m1 --h3 -H ":authority: $HOST" "$url" >/dev/null 2>&1 ;;
	h3bench) "$H3BENCH_CMD" -d 1s -c 1 -dial "$dial_addr" -k "$url" >/dev/null 2>&1 ;;
	esac
}

enabled_protocols_label() {
	local protocols=()
	[ "$H1" = "1" ] && protocols+=("HTTP/1.1")
	[ "$H2" = "1" ] && protocols+=("HTTP/2")
	[ "$H3" = "1" ] && protocols+=("HTTP/3")
	printf "%s" "${protocols[0]}"
	printf ", %s" "${protocols[@]:1}"
	echo ""
}

# Array to store connection errors
declare -a connection_errors=()

# Function to test connection before benchmarking
test_connection() {
	local name=$1
	local url=$2

	yellow "Testing connection to $name..."

	local h3_url
	h3_url=$(h3_url "$name")

	local failed=false
	if [ "$H1" = "1" ]; then
		local res1=$(read_response_with_retry --http1.1 "$url")
		local body1=$(echo "$res1" | head -n -1)
		local status1=$(echo "$res1" | tail -n 1)

		if [ "$status1" != "200" ] || [ ${#body1} -ne 4096 ]; then
			red "✗ $name failed HTTP/1.1 connection test (Status: $status1, Body length: ${#body1})"
			failed=true
		fi
	fi

	if [ "$H2" = "1" ]; then
		local res2=$(read_response_with_retry --http2-prior-knowledge "$url")
		local body2=$(echo "$res2" | head -n -1)
		local status2=$(echo "$res2" | tail -n 1)

		if [ "$status2" != "200" ] || [ ${#body2} -ne 4096 ]; then
			red "✗ $name failed HTTP/2 connection test (Status: $status2, Body length: ${#body2})"
			failed=true
		fi
	fi

	if [ "$H3" = "1" ] && [ -n "$h3_url" ]; then
		if ! h3_check_with_retry "$name" "$h3_url"; then
			red "✗ $name failed HTTP/3 connection test (URL: $h3_url)"
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

# Function to run benchmark
run_benchmark() {
	local name=$1
	local url=$2
	local h3_url
	h3_url=$(h3_url "$name")
	local h3_dial_addr
	h3_dial_addr=$(h3_dial_addr "$name")
	local h2_duration="${DURATION%s}"

	restart_bench "$name"

	yellow "Testing $name..."

	echo "========================================"
	echo "$name"
	echo "URL: $url"
	echo "========================================"
	echo ""
	if [ "$H1" = "1" ]; then
		echo "[HTTP/1.1] wrk"

		wrk -t"$THREADS" -c"$CONNECTIONS" -d"$DURATION" \
			-H "Host: $HOST" \
			"$url"
	fi

	if [ "$H2" = "1" ]; then
		echo ""
		restart_bench "$name"

		echo "[HTTP/2] h2load"

		h2load -t"$THREADS" -c"$CONNECTIONS" --duration="$h2_duration" \
			-H "Host: $HOST" \
			-H ":authority: $HOST" \
			"$url" | grep -vE "^(starting benchmark...|spawning thread|progress: |Warm-up |Main benchmark duration|Stopped all clients|Process Request Failure)"
	fi

	if [ "$H3" = "1" ]; then
		echo ""
		echo "[HTTP/3] $H3_TOOL"
		restart_bench "$name"

		case "$H3_TOOL" in
			h2load)
				h2load -t"$THREADS" -c"$CONNECTIONS" --duration="$h2_duration" \
					-k \
					--h3 \
					-H ":authority: $HOST" \
					"$h3_url" | grep -vE "^(starting benchmark...|spawning thread|progress: |Warm-up |Main benchmark duration|Stopped all clients|Process Request Failure)"
				;;
			h3bench)
				"$H3BENCH_CMD" -d "$DURATION" -c "$CONNECTIONS" -dial "$h3_dial_addr" -k "$h3_url"
				;;
		esac
	fi

	echo ""
	green "✓ $name benchmark completed"
	blue "----------------------------------------"
	echo ""
}

# Run benchmarks for each service
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
echo "Key metrics to compare:"
echo "  - Requests/sec (throughput)"
echo "  - Latency (mean, stdev)"
echo "  - Transfer/sec"
if [ "$H3" = "1" ]; then
	echo "  - HTTP/3 QUIC stats (RTT, packets sent/recv/lost)"
fi
echo ""
green "All benchmarks completed!"
