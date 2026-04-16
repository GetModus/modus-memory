#!/bin/zsh
set -euo pipefail

ROOT="/Users/modus/modus"
SRC_DIR="$ROOT/scripts"
BIN_DIR="${1:-$HOME/.local/bin}"

mkdir -p "$BIN_DIR"

wrappers=(
  modus-attach-carrier
  modus-codex
  modus-qwen
  modus-gemini
  modus-ollama
  modus-hermes
  modus-openclaw
  modus-opencode
)

for wrapper in "${wrappers[@]}"; do
  ln -sfn "$SRC_DIR/$wrapper" "$BIN_DIR/$wrapper"
done

echo "Installed MODUS Memory attachment wrappers to $BIN_DIR"
echo "Active commands:"
for wrapper in "${wrappers[@]}"; do
  echo "  $wrapper"
done
echo
echo "Claude is intentionally not installed as a live wrapper because the carrier estate is closed."
