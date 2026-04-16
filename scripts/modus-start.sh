#!/bin/zsh
set -euo pipefail

ROOT="/Users/modus/modus"
GO_DIR="$ROOT/go"
LOG_DIR="$ROOT/logs"
GOCACHE_DIR="$ROOT/.gocache"

mkdir -p "$LOG_DIR" "$GOCACHE_DIR"
cd "$GO_DIR"

if [[ -x "$GO_DIR/modus" ]]; then
  exec env GOCACHE="$GOCACHE_DIR" "$GO_DIR/modus" start
fi

exec env GOCACHE="$GOCACHE_DIR" go run ./cmd/modus start
