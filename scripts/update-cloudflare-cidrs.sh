#!/bin/sh
set -eu

out="${1:-internal/net/gphttp/middleware/cloudflare_real_ip_seed.txt}"
tmp="$(mktemp)"
trap 'rm -f "$tmp"' EXIT

for endpoint in \
	https://www.cloudflare.com/ips-v4 \
	https://www.cloudflare.com/ips-v6
do
	curl -fsSL "$endpoint"
	printf '\n'
done | awk 'NF { print }' > "$tmp"

if [ ! -s "$tmp" ]; then
	echo "cloudflare CIDR snapshot is empty" >&2
	exit 1
fi

cp "$tmp" "$out"
