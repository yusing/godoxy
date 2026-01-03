#!/bin/bash
# Benchmark script to compare GoDoxy, Traefik, Caddy, and Nginx
# Uses wrk for HTTP load testing

set -e

# Configuration
HOST="bench.domain.com"
DURATION="${DURATION:-10s}"
THREADS="${THREADS:-4}"
CONNECTIONS="${CONNECTIONS:-100}"
TARGET="${TARGET-}"

# Color functions for output
red() { echo -e "\033[0;31m$*\033[0m"; }
green() { echo -e "\033[0;32m$*\033[0m"; }
yellow() { echo -e "\033[1;33m$*\033[0m"; }
blue() { echo -e "\033[0;34m$*\033[0m"; }

# Check if wrk is installed
if ! command -v wrk &>/dev/null; then
	red "Error: wrk is not installed"
	echo "Please install wrk:"
	echo "  Ubuntu/Debian: sudo apt-get install wrk"
	echo "  macOS: brew install wrk"
	echo "  Or build from source: https://github.com/wg/wrk"
	exit 1
fi

if ! command -v h2load &>/dev/null; then
	red "Error: h2load is not installed"
	echo "Please install h2load (nghttp2-client):"
	echo "  Ubuntu/Debian: sudo apt-get install nghttp2-client"
	echo "  macOS: brew install nghttp2"
	exit 1
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

# Array to store connection errors
declare -a connection_errors=()

# Function to test connection before benchmarking
test_connection() {
	local name=$1
	local url=$2

	yellow "Testing connection to $name..."

	# Test HTTP/1.1
	local res1=$(curl -sS -w "\n%{http_code}" --http1.1 -H "Host: $HOST" --max-time 5 "$url")
	local body1=$(echo "$res1" | head -n -1)
	local status1=$(echo "$res1" | tail -n 1)

	# Test HTTP/2
	local res2=$(curl -sS -w "\n%{http_code}" --http2-prior-knowledge -H "Host: $HOST" --max-time 5 "$url")
	local body2=$(echo "$res2" | head -n -1)
	local status2=$(echo "$res2" | tail -n 1)

	local failed=false
	if [ "$status1" != "200" ] || [ ${#body1} -ne 4096 ]; then
		red "✗ $name failed HTTP/1.1 connection test (Status: $status1, Body length: ${#body1})"
		failed=true
	fi

	if [ "$status2" != "200" ] || [ ${#body2} -ne 4096 ]; then
		red "✗ $name failed HTTP/2 connection test (Status: $status2, Body length: ${#body2})"
		failed=true
	fi

	if [ "$failed" = true ]; then
		connection_errors+=("$name failed connection test (URL: $url)")
		return 1
	else
		green "✓ $name is reachable (HTTP/1.1 & HTTP/2)"
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
	yellow "Restarting bench service before benchmarking $name HTTP/1.1..."
	docker compose -f dev.compose.yml up -d --force-recreate bench >/dev/null 2>&1
	sleep 1
}

# Function to run benchmark
run_benchmark() {
	local name=$1
	local url=$2
	local h2_duration="${DURATION%s}"

	restart_bench "$name"

	yellow "Testing $name..."

	echo "========================================"
	echo "$name"
	echo "URL: $url"
	echo "========================================"
	echo ""
	echo "[HTTP/1.1] wrk"

	wrk -t"$THREADS" -c"$CONNECTIONS" -d"$DURATION" \
		-H "Host: $HOST" \
		"$url"

	restart_bench "$name"

	echo ""
	echo "[HTTP/2] h2load"

	h2load -t"$THREADS" -c"$CONNECTIONS" --duration="$h2_duration" \
		-H "Host: $HOST" \
		-H ":authority: $HOST" \
		"$url" | grep -vE "^(starting benchmark...|spawning thread|progress: |Warm-up |Main benchmark duration|Stopped all clients|Process Request Failure)"

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
echo "Key metrics to compare:"
echo "  - Requests/sec (throughput)"
echo "  - Latency (mean, stdev)"
echo "  - Transfer/sec"
echo ""
green "All benchmarks completed!"
