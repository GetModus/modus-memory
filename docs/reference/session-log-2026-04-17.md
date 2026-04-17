---
title: Homing Session Log
date: 2026-04-17
status: active
project: Homing by MODUS
release: v0.6.0
---

# Homing Session Log — 2026-04-17

This log records the work completed for Homing by MODUS on April 17, 2026.

## Objectives For The Session

- finish the capture-policy work in the standalone repository
- make the rename to `homing` operational instead of merely cosmetic
- verify the runtime honestly
- merge the changes into `main`
- publish a proper `v0.6.0` release
- polish the public-facing docs and launch copy

## What We Changed

### 1. Promoted `homing` to the primary binary

- added `cmd/homing`
- kept `cmd/modus-memory` as a compatibility alias
- moved the shared CLI logic into `internal/app` so both binaries run the same code path

### 2. Added policy-driven turn capture

- added `memory_capture` to the MCP tool surface
- supported admission policies:
  - `minimal`
  - `balanced`
  - `strict`
  - `everything`
- supported `dry_run: true`
- allowed the tool to decide whether a turn becomes:
  - `skip`
  - `episode_only`
  - `facts_only`
  - `episode_and_facts`

### 3. Strengthened verification

- added MCP protocol coverage for:
  - `initialize`
  - `tools/list`
  - `tools/call`
- added direct tests around `memory_capture`
- ran full repository tests
- built both `homing` and `modus-memory`
- smoke-checked binary version output, health output, and MCP stdio behavior

### 4. Fixed real product inconsistencies

- corrected the standalone default vault path to `~/vault`
- added `HOMING_VAULT_DIR`
- kept `MODUS_VAULT_DIR` as a compatibility alias
- removed stale `0.3.0` strings from shipped SVG assets
- aligned docs with the actual standalone behavior

### 5. Merged and released

- merged PR `#7` into `main`
- published GitHub release `v0.6.0`
- attached release binaries:
  - `homing-darwin-arm64`
  - `homing-darwin-amd64`
  - `homing-linux-amd64`
  - `homing-linux-arm64`
  - `homing-windows-amd64.exe`
  - `checksums.txt`

### 6. Polished public-facing materials

- rewrote the GitHub release body so it reads like a release note instead of an internal document
- tightened the top of the README with:
  - `At A Glance`
  - `What Changed In v0.6.0`
  - `Fastest start`
- created reusable launch messaging packs

## Files Added Or Updated

- `README.md`
- `cmd/modus-memory/README.md`
- `cmd/homing/README.md`
- `cmd/homing/main.go`
- `internal/app/*`
- `internal/mcp/vault.go`
- `internal/mcp/memory.go`
- `internal/mcp/server_protocol_test.go`
- `internal/mcp/vault_capture_test.go`
- `internal/index/indexer.go`
- `docs/reference/release-notes-v0.6.0-homing.md`
- `docs/reference/homing-memory-update-2026-04.md`
- `docs/reference/session-log-2026-04-17.md`
- `docs/research/homing-v0.6.0-launch-copy.md`
- `docs/research/homing-v0.6.0-social-and-outreach-pack.md`
- `assets/demo.svg`
- `assets/doctor.svg`

## Verification Performed

```bash
GOCACHE=/tmp/modus-memory/.gocache GOMODCACHE=/tmp/modus-memory/.gomodcache go test ./...
GOCACHE=/tmp/modus-memory/.gocache GOMODCACHE=/tmp/modus-memory/.gomodcache go build ./cmd/homing
GOCACHE=/tmp/modus-memory/.gocache GOMODCACHE=/tmp/modus-memory/.gomodcache go build ./cmd/modus-memory
```

Additional smoke checks performed:

- `homing version`
- `modus-memory version`
- `homing --vault <tmp> health`
- MCP `initialize`, `tools/list`, and `tools/call` against the built binary
- default standalone vault path verification using a temporary `HOME`

## Outcome

At the end of the session:

- `main` contains the rename, capture-policy work, vault-path fix, and docs updates
- `v0.6.0` is published and downloadable
- the public-facing copy is materially cleaner
- launch and outreach copy now exists in-repo for reuse

## Follow-Up Candidates

- produce final platform-specific social copy variants if tone needs to differ by audience
- add a short demo video optimized specifically for release pages and social clips
- decide whether the compatibility alias `modus-memory` should remain indefinitely or receive a timed deprecation plan
