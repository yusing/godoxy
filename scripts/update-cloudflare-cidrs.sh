#!/usr/bin/env bash
set -euo pipefail

out="${1:-internal/net/gphttp/middleware/cloudflare_real_ip_seed.txt}"
tmp_raw="$(mktemp)"
tmp="$(mktemp)"
trap 'rm -f "$tmp_raw" "$tmp"' EXIT

: > "$tmp_raw"
for endpoint in \
	https://www.cloudflare.com/ips-v4 \
	https://www.cloudflare.com/ips-v6
do
	curl -fsSL "$endpoint" >> "$tmp_raw"
	printf '\n' >> "$tmp_raw"
done

awk 'NF { print }' "$tmp_raw" > "$tmp"

if [ ! -s "$tmp" ]; then
	echo "cloudflare CIDR snapshot is empty" >&2
	exit 1
fi

cp "$tmp" "$out"
