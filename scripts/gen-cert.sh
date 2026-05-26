#!/bin/bash
set -e

CERT_DIR="${1:-certs}"
mkdir -p "$CERT_DIR"

openssl req -x509 -newkey rsa:4096 \
  -keyout "$CERT_DIR/server.key" \
  -out "$CERT_DIR/server.crt" \
  -days 365 -nodes \
  -subj "/CN=localhost/O=QQGO/C=CN"

chmod 600 "$CERT_DIR/server.key"
chmod 644 "$CERT_DIR/server.crt"

echo "Certificates generated in $CERT_DIR/"
echo "  $CERT_DIR/server.crt"
echo "  $CERT_DIR/server.key"
