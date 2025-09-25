#!/usr/bin/env bash
set -euo pipefail
DIST=${1:-dist}
mkdir -p "$DIST"

# Windows amd64
GOOS=windows GOARCH=amd64 go build -o "$DIST/cli-server-windows-amd64.exe" ./cmd/cli-server
GOOS=windows GOARCH=amd64 go build -o "$DIST/cli-client-windows-amd64.exe" ./cmd/cli-client

# Linux amd64
GOOS=linux GOARCH=amd64 go build -o "$DIST/cli-server-linux-amd64" ./cmd/cli-server
GOOS=linux GOARCH=amd64 go build -o "$DIST/cli-client-linux-amd64" ./cmd/cli-client

echo "CLI builds written to $DIST"