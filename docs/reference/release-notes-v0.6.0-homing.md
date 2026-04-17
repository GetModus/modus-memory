---
title: Homing by MODUS Release Notes
version: 0.6.0
date: 2026-04-17
status: active
product: Homing by MODUS
binary_name: homing
compatibility_binary_name: modus-memory
---

# Homing by MODUS — Release Notes for v0.6.0

`v0.6.0` is the release where the rename becomes operational.

The public story changed in `v0.5.0`. This release carries that change through the command surface itself. The primary binary is now `homing`. The compatibility alias `modus-memory` remains buildable and supported for existing scripts, wrappers, and MCP configurations.

## Executive Summary

Homing now presents one cleaner story to users and buyers:

- the product name is Homing by MODUS
- the primary command is `homing`
- the legacy alias is `modus-memory`
- the module path remains `github.com/GetModus/modus-memory`

At the same time, this release addresses the biggest adoption trap we saw in the field: MCP connectivity does not automatically mean memory admission. The new `memory_capture` tool gives desktop clients and harnesses one policy-governed call that can decide whether to store an episode, facts, both, or nothing.

## What Is New In v0.6.0

### Primary binary rename

The repo now builds two entrypoints:

- `cmd/homing` as the primary binary
- `cmd/modus-memory` as the compatibility alias

This keeps new installs clean without breaking existing operators who already have `modus-memory` wired into a client.

### Policy-driven turn capture

The MCP surface now includes `memory_capture`.

It accepts a compact turn summary, optional explicit facts, and an admission policy:

- `minimal`
- `balanced`
- `strict`
- `everything`

The tool can then decide whether the turn should become:

- `skip`
- `episode_only`
- `facts_only`
- `episode_and_facts`

It also supports `dry_run: true`, which is useful when integrating a new client rule and wanting proof of the policy decision without writing anything.

### Cleaner MCP integration guidance

This release makes an explicit distinction between:

- clients that can mount an MCP server and call tools directly
- shells or harnesses that need sovereign attachment instead

The documentation now says plainly that a connected MCP client must still call `memory_capture`, `memory_store`, or `memory_episode_store` if it wants durable writes.

## Runtime Surface

The standalone Homing MCP server now exposes:

- 28 MCP tools available to every user

The surface includes vault access, search, episodic store, policy-driven capture, traces, governance proposals, maintenance, secure state, readiness, evaluation, trials, portability, reinforcement, decay, tuning, training, and connected graph queries.

## Naming And Compatibility

The compatibility posture is intentionally conservative:

- primary command is now `homing`
- compatibility alias remains `modus-memory`
- module path remains `github.com/GetModus/modus-memory`
- existing MCP integrations that still target `modus-memory --vault ...` continue to work
- no license activation is required

## Client And Harness Posture

This release does not pretend every client is the same.

Desktop MCP clients such as Claude Desktop, Cursor, Windsurf, Codex app, Antigravity, Claude Code, and similar tool-aware shells depend on the same protocol contract:

- `initialize`
- `tools/list`
- `tools/call`

Homing was verified against that transport contract directly. Whether a particular client exposes the settings UI elegantly is a product question on their side, not a protocol question on ours.

For plain shells and CLIs, the supported lane remains sovereign attachment through:

- `homing attach --carrier ...`
- the MODUS wrapper commands

## Documentation Updated In This Release

- [README.md](../../README.md)
- [CHANGELOG.md](../../CHANGELOG.md)
- [docs/reference/homing-memory-update-2026-04.md](./homing-memory-update-2026-04.md)
- [docs/research/modus-memory-partner-brief-2026-04-16.md](../research/modus-memory-partner-brief-2026-04-16.md)

## Verification

The following verification was run for this release line:

```bash
GOCACHE=/tmp/modus-memory/.gocache GOMODCACHE=/tmp/modus-memory/.gomodcache go test ./...
GOCACHE=/tmp/modus-memory/.gocache GOMODCACHE=/tmp/modus-memory/.gomodcache go build ./cmd/homing
GOCACHE=/tmp/modus-memory/.gocache GOMODCACHE=/tmp/modus-memory/.gomodcache go build ./cmd/modus-memory
```

The repository now has:

- full Go test coverage passing
- protocol-level MCP coverage for `initialize`, `tools/list`, and `tools/call`
- both primary and compatibility binaries building cleanly

Exact binary size still varies by target platform, architecture, and build flags.
