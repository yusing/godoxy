#!/usr/bin/env bash
set -eu

HOST="${HOST:-bench.domain.com}"
CERT_DIR="${CERT_DIR:-dev-data/certs}"
CERT_FILE="$CERT_DIR/$HOST.crt"
KEY_FILE="$CERT_DIR/$HOST.key"

mkdir -p "$CERT_DIR"

if [[ -s "$CERT_FILE" && -s "$KEY_FILE" ]] && \
	openssl x509 -in "$CERT_FILE" -noout -checkend 86400 >/dev/null 2>&1 && \
	openssl x509 -in "$CERT_FILE" -noout -ext subjectAltName 2>/dev/null | grep -q "DNS:$HOST"; then
	exit 0
fi

openssl req -x509 -newkey rsa:2048 -sha256 -days 30 -nodes \
	-subj "/CN=$HOST" \
	-addext "subjectAltName=DNS:$HOST" \
	-keyout "$KEY_FILE" \
	-out "$CERT_FILE" >/dev/null 2>&1

chmod 600 "$KEY_FILE"
chmod 644 "$CERT_FILE"
