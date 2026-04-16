#!/bin/zsh
set -euo pipefail

ROOT="/Users/modus/modus"
GO_DIR="$ROOT/go"
MODUS_BIN="$GO_DIR/modus"
GOCACHE_DIR="$ROOT/.gocache"

print_usage() {
  cat <<'EOF'
Usage:
  modus-attach-carrier <carrier> [attachment flags] [prompt words...]

Active carriers on this machine:
  codex, qwen, gemini, ollama, hermes, openclaw, opencode

Supported but presently inactive:
  claude

Behavior:
  - if --prompt is not provided, remaining positional arguments become the prompt
  - if no prompt arguments are provided, stdin is forwarded to MODUS Memory
  - --workdir defaults to the current directory
  - openclaw defaults to --target main unless --target or MODUS_OPENCLAW_TARGET is set

Examples:
  modus-attach-carrier codex "Summarize the current task."
  modus-attach-carrier opencode --json "Reply with exactly: nominal."
  modus-attach-carrier openclaw --target main "Reply with exactly: nominal."
EOF
}

if (( $# == 0 )); then
  print_usage >&2
  exit 1
fi

carrier="$1"
shift

if [[ "$carrier" == "--help" || "$carrier" == "-h" ]]; then
  print_usage
  exit 0
fi

typeset -a forward_args
typeset -a prompt_parts
prompt_explicit=0
workdir_explicit=0
target_explicit=0

while (( $# > 0 )); do
  case "$1" in
    --help|-h)
      print_usage
      exit 0
      ;;
    --json|--no-episode|--ephemeral)
      forward_args+=("$1")
      shift
      ;;
    --prompt|--model|--workdir|--target|--subject|--work-item-id|--recall-limit)
      flag="$1"
      shift
      if (( $# == 0 )); then
        echo "missing value for $flag" >&2
        exit 1
      fi
      if [[ "$flag" == "--prompt" ]]; then
        prompt_explicit=1
      fi
      if [[ "$flag" == "--workdir" ]]; then
        workdir_explicit=1
      fi
      if [[ "$flag" == "--target" ]]; then
        target_explicit=1
      fi
      forward_args+=("$flag" "$1")
      shift
      ;;
    --)
      shift
      while (( $# > 0 )); do
        prompt_parts+=("$1")
        shift
      done
      ;;
    -*)
      echo "unsupported wrapper flag: $1" >&2
      exit 1
      ;;
    *)
      prompt_parts+=("$1")
      shift
      ;;
  esac
done

if (( ! workdir_explicit )); then
  forward_args+=(--workdir "${MODUS_ATTACH_WORKDIR:-$PWD}")
fi

if [[ "$carrier" == "openclaw" && $target_explicit -eq 0 ]]; then
  forward_args+=(--target "${MODUS_OPENCLAW_TARGET:-main}")
fi

if (( ! prompt_explicit )) && (( ${#prompt_parts[@]} > 0 )); then
  forward_args+=(--prompt "${(j: :)prompt_parts}")
fi

mkdir -p "$GOCACHE_DIR"

if [[ ! -x "$MODUS_BIN" ]]; then
  (cd "$GO_DIR" && env GOCACHE="$GOCACHE_DIR" go build -o "$MODUS_BIN" ./cmd/modus)
fi

exec env GOCACHE="$GOCACHE_DIR" "$MODUS_BIN" memory attach --carrier "$carrier" "${forward_args[@]}"
