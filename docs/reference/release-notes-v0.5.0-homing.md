---
title: Homing by MODUS Release Notes
version: 0.5.0
date: 2026-04-16
status: active
product: Homing by MODUS
binary_name: modus-memory
---

# Homing by MODUS — Release Notes for v0.5.0

`v0.5.0` is the first release in the Homing by MODUS line.

The headline change is not merely branding. `modus-memory` now ships a broader sovereign memory runtime with route-aware retrieval, first-class episodes, durable recall receipts, governed review flows, shell-first carrier attachment, portability auditing, secure-state verification, readiness reporting, and both synthetic and live evaluation surfaces.

The binary name remains `modus-memory`. The public product name is **Homing by MODUS**.

## Executive Summary

Before this release, the product story still read like a compact MCP memory server.

After this release, the product is better described as a local-first memory kernel for agents. MCP remains one adapter. Shell wrappers remain another. The key shift is that memory is now treated as governed system state rather than a thin search-and-store service.

## What Is New In v0.5.0

### Route-aware recall

Retrieval can now narrow through route selectors before final ranking, including:

- subject
- mission
- office
- work item
- lineage
- environment
- time band
- cue terms

### Episodic identity

The runtime now supports first-class episodic memory objects with durable identity fields such as:

- `event_id`
- `lineage_id`
- `content_hash`
- `event_kind`
- `cue_terms`

### Recall receipts and traces

Recall is now auditable. The system can write durable receipts describing:

- the query
- the harness
- the adapter
- selected paths
- linked memory surfaces
- verification state when critical recall is requested

### Governed review

Memory mutation is no longer treated as informal file surgery. The runtime now supports explicit review artifacts for:

- hot-tier transitions
- temporal transitions
- elder-memory protection
- structural linkage
- replay-driven promotion

### Shell-first sovereign attachment

The standalone repo now ships wrapper scripts and attachment flows so plain shell carriers can use governed memory without exposing native memory tools.

### Assurance surfaces

The memory subsystem now includes:

- secure-state manifest generation and verification
- portability audit
- readiness reporting
- synthetic evaluation
- live trial runs

## Runtime Surface

The standalone `modus-memory` server now exposes:

- 22 core MCP tools
- 5 Pro extensions

Core tools cover vault access, memory search and store, episodic store, traces, governance proposals, maintenance, secure state, readiness, evaluation, trials, and portability.

Pro extensions currently cover:

- `memory_reinforce`
- `memory_decay_facts`
- `memory_tune`
- `memory_train`
- `vault_connected`

## Naming And Compatibility

The product is now presented publicly as **Homing by MODUS**.

Compatibility remains intentionally conservative:

- command name stays `modus-memory`
- module path stays `github.com/GetModus/modus-memory`
- existing MCP integrations still target `modus-memory --vault ...`

## Documentation Updated In This Release

- [README.md](../../README.md)
- [CHANGELOG.md](../../CHANGELOG.md)
- [docs/reference/homing-memory-update-2026-04.md](./homing-memory-update-2026-04.md)
- [docs/research/modus-memory-partner-brief-2026-04-16.md](../research/modus-memory-partner-brief-2026-04-16.md)

## Verification

The following verification was run for the release-polish pass:

```bash
GOCACHE=/tmp/modus-memory-gocache go test ./...
GOCACHE=/tmp/modus-memory-gocache go build -ldflags="-s -w" -o /tmp/modus-memory-stripped.bin ./cmd/modus-memory
```

Representative release fact:

- stripped Apple Silicon build size is approximately `7.7 MB`

Exact binary size varies by platform, architecture, and build flags.
