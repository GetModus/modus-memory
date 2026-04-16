---
title: Homing Main Merge Summary - April 2026
date: 2026-04-16
status: active
target: main
branch: codex/homing-main-port
---

# Homing Main Merge Summary - April 2026

This document is the merge-ready summary for bringing the Homing branch into `main`.

It exists to answer four reviewer questions quickly:

1. What actually landed
2. Which Grade S memory changes are included
3. How users are expected to set it up and use it
4. What verification has already been run

## Branch State

Target branch for review:

`codex/homing-main-port`

Representative commits on top of the branch:

- `docs: launch Homing by MODUS branding pass`
- `feat: land Homing memory kernel and governance update`
- `docs: track Homing partner brief and launch posts`

## What This Merge Brings Into Main

This merge brings the recent memory campaign from concept and doctrine into the live codebase.

At a high level, it turns MODUS Memory into **Homing by MODUS**, a sovereign memory kernel with:

- route-aware recall
- episodic memory identity
- durable recall receipts
- governed review flows
- shell-first sovereign attachment
- source verification
- temporal truth support
- elder-memory review
- replay promotion
- secure-state auditing
- portability auditing
- readiness, trials, and evaluation

## Grade S Changes Included

These are the major Grade S memory changes now included in the branch.

### 1. Memory kernel

The branch introduces the direct memory kernel under `internal/memorykit/`.

This is the core architectural change. Memory now has a local authority layer instead of being treated as an MCP-only product surface.

Included:

- direct memory kernel
- attached carrier execution
- carrier audit and live probe flows
- portability audit
- secure-state manifest and verification
- readiness report
- live trial harness
- synthetic evaluation harness

### 2. Episodic identity

The branch introduces first-class episodes under `internal/vault/episodes.go`.

Included:

- `event_id`
- `lineage_id`
- `content_hash`
- `event_kind`
- `cue_terms`
- episode provenance and relation fields

### 3. Recall receipts

The branch introduces first-class recall receipts under `internal/vault/recalls.go`.

Included:

- durable record of recall query and adapter
- selected paths
- linked memory surfaces
- reinforcement on successful recall
- receipt artifacts in the vault

### 4. Route-aware retrieval

The branch extends fact recall and ranking in `internal/vault/facts.go`.

Included route selectors:

- subject
- mission
- office
- cue terms
- time band
- work item
- lineage
- environment

### 5. Temporal truth and verification

The branch adds:

- temporal status support
- `observed_at`, `valid_from`, `valid_to`
- critical source verification
- correction propagation improvements

Primary files:

- `internal/vault/facts.go`
- `internal/vault/verification.go`
- `internal/vault/corrections.go`
- `internal/maintain/temporal.go`

### 6. Governed review and maintenance

The branch adds explicit review flows under `internal/maintain/` and `internal/memorycli/`.

Included:

- hot-tier review
- structural review
- temporal review
- elder-memory review
- replay promotion review
- review queue inspection
- review resolution and apply flows

### 7. Shell-first memory use

The branch adds stable wrapper-based attachment paths and shell governance commands.

Included:

- `scripts/install-memory-attach-wrappers.sh`
- `modus-attach-carrier`
- `modus-codex`
- `modus-qwen`
- `modus-gemini`
- `modus-ollama`
- `modus-hermes`
- `modus-openclaw`
- `modus-opencode`

### 8. Surface integration

The branch wires the memory update through the rest of the product.

Included integration surfaces:

- agent session and registry
- heartbeat and session prep
- TUI launch path
- server and operator shell
- `modus` and `modus-memory` command entry points

## What Users Will Notice

For MCP-capable clients, setup still begins with `modus-memory --vault ...`, but memory behavior is now richer and better-governed.

For shell-native carriers, the major user-visible change is that sovereign attachment is now a first-class path instead of an improvised one.

For stewards and operators, the major change is that memory proposals and review are now explicit and inspectable.

For reviewers and maintainers, the major change is that memory quality is now supported by trials, evaluation, readiness, portability, and secure-state reports.

## Docs Added For This Merge

These docs are now part of the branch and should be treated as the primary reviewer and user guides for the merge:

- [cmd/modus-memory/README.md](../../cmd/modus-memory/README.md)
- [README.md](../../README.md)
- [docs/reference/homing-memory-update-2026-04.md](./homing-memory-update-2026-04.md)
- [docs/research/modus-memory-partner-brief-2026-04-16.md](../research/modus-memory-partner-brief-2026-04-16.md)

## Verification Already Run

The following targeted verification was run successfully before pushing the curated branch update:

```bash
GOCACHE=/tmp/modus-memory-gocache go test ./...
GOCACHE=/tmp/modus-memory-gocache go build ./cmd/modus-memory
```

The package test run passed for the standalone Homing runtime, including:

- `internal/memorykit`
- `internal/memorycli`
- `internal/maintain`
- `internal/vault`
- `internal/mcp`
- `internal/trainer`
- `internal/trust`

The standalone binary also built successfully:

- `cmd/modus-memory`

## Remaining Caveat Before Main Merge

The branch itself is cleanly pushed and contains the curated memory work, but the local working tree still has unrelated unstaged changes outside the memory project.

That does **not** block the branch merge.

It only means the merge should be taken from the pushed branch commits, not from the current local filesystem state.

## Recommended Merge Framing

If this goes into `main`, the clearest framing is:

“Land Homing memory kernel and governance update. Introduce route-aware recall, episodic identity, recall receipts, shell-first sovereign attachment, governed maintenance flows, verification, portability, and readiness surfaces, with public-facing Homing branding and updated setup documentation.”

That is a truthful description of what this branch now contains.
