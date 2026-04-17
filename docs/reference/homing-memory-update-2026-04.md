---
title: Homing Memory Update - April 2026
date: 2026-04-16
status: active
product: Homing by MODUS
binary_name: homing
compatibility_binary_name: modus-memory
---

# Homing Memory Update - April 2026

This document explains the current Homing update in plain language: what changed, what is better, what was fixed, and how to set it up and use it without needing to reverse-engineer the code.

The public product name is **Homing by MODUS**.

The current shipped primary binary and command name are now **`homing`**.

The legacy compatibility alias remains **`modus-memory`**.

## What Homing Is Now

Homing is no longer just a local memory server for AI clients.

It is now a sovereign memory kernel for agents. That means memory is treated as a local system capability with explicit rules for what gets stored, how it is recalled, how it is reviewed, how it is protected, and how it can be attached to clients that do not have native memory tools of their own.

In practical terms, the system now supports both:

- true MCP clients that can call memory tools directly
- plain shells, CLIs, and harnesses that need memory attached externally

## What We Made Better

### 1. Memory is now route-aware instead of flat

Recall no longer behaves like one undifferentiated search through old text.

The system can now narrow by:

- subject
- mission
- office
- work item
- lineage
- cue terms
- time band
- environment

This is the core of the new “homing” behavior. Memory returns through route and cue instead of only semantic similarity.

### 2. Memory now has episodic identity

We added first-class episodic memory objects under `vault/memory/episodes/`.

Episodes carry:

- `event_id`
- `lineage_id`
- `content_hash`
- `event_kind`
- `cue_terms`

This makes memory less blur-prone. Similar experiences no longer need to collapse into one vague semantic claim.

### 3. Recall now leaves receipts

Retrieval is now a durable action rather than a transient courtesy.

Recall receipts under `vault/memory/recalls/` record:

- the query
- the harness
- the adapter
- the recall mode
- the selected results
- linked facts and episodes
- verification state when critical recall is requested

This makes memory auditable and gives us proof of what the system actually consulted.

### 4. Memory now has a direct kernel

The core memory behavior now lives behind a direct local kernel in `internal/memorykit/`.

That matters because MCP is no longer the only honest integration path. The same core can now serve:

- MCP clients
- shell wrappers
- internal registry integrations
- TUI and server surfaces

### 5. Shell-native agents can now use sovereign memory

This is one of the most important practical improvements.

A lot of useful carriers are plain shells, not tool-native clients. Homing now supports a sovereign attachment lane that:

1. recalls hot memory
2. augments the prompt
3. runs the carrier
4. writes a recall receipt
5. writes a trace
6. optionally writes an episode

That means a shell can participate in governed memory even if it never exposes `memory_search` or `memory_store`.

### 6. Memory governance is now explicit

The system now has shell-first governance flows for:

- hot-memory promotion and demotion
- structural link proposals
- temporal status proposals
- elder-memory proposals
- review queue inspection
- explicit review resolution

Memory changes no longer need to happen as silent mutation or hand-edited markdown surgery.

### 7. Memory now tracks temporal truth better

Facts now support time semantics beyond `created_at`, including:

- `observed_at`
- `valid_from`
- `valid_to`
- temporal status such as active, superseded, or expired

This makes memory more honest about when something was true, not just when it was written down.

### 8. Critical recall can now verify sources

For higher-stakes recall, the system can reopen cited sources and annotate results as:

- verified
- review required
- mismatch
- unverified
- source missing

That moves the system closer to evidence-bearing recall rather than merely confident recall.

### 9. Long-horizon memory protection is now part of the design

The system now has elder-memory review logic for rare, high-consequence knowledge that should not be buried by recency bias.

This is especially important for commitments, identity, mission-critical context, and exceptional historical lessons.

### 10. The system can now score itself

Homing now includes:

- authored live memory trials
- synthetic memory evaluation
- portability audits
- secure-state verification
- readiness reporting

That means we can do more than describe the architecture. We can inspect whether it is actually behaving.

## What We Fixed

The update did not just add new surfaces. It also addressed several structural weaknesses that would have become more dangerous over time.

We fixed the tendency for memory retrieval to behave too much like generic search rather than route-aware recall.

We fixed the absence of durable evidence for retrieval itself by introducing recall receipts.

We fixed the overreliance on flat semantic memory by introducing episodes and lineage-bearing identity.

We fixed the previous ambiguity around hot versus warm memory by continuing the governed hot-tier model and extending the stewardship flows around it.

We fixed the gap between tool-native clients and plain shells by introducing the sovereign attachment lane and stable wrapper scripts.

We fixed part of the memory-sovereignty gap by adding portability audits over provider-side memory residue.

We fixed part of the trust gap by adding source verification and secure-state drift and rollback checks.

## What Changed For Users

If you use Homing through an MCP-capable client, the core setup is still simple: point the client at `homing --vault ...`.

But the caveat is important: MCP connectivity alone does not produce automatic memory admission. If the client does not call the write tools, nothing durable is stored.

If you use shell-native agents, the major change is that you should now prefer the attachment wrappers rather than trying to force every carrier into pretending it is a direct memory client.

If you steward memory directly, you now have explicit proposal and review commands instead of relying on ad hoc edits.

If you care about memory quality, you now have live reports and evaluation artifacts to inspect.

## Setup

### Option 1: True MCP client

Use this when the client can mount a stdio MCP server and call memory tools directly.

Example:

```bash
homing --vault ~/vault
```

Typical clients:

- Claude Desktop
- Claude Code in MCP mode
- Cursor in MCP mode
- Windsurf in MCP mode
- Codex app in MCP mode
- Antigravity in MCP mode
- any agent framework with real stdio MCP support

For Cursor, add an explicit memory policy. The recommended pattern is to call `memory_capture` at the end of every turn with `policy: balanced`, and to use `dry_run: true` when validating the policy without writing.

### Option 2: Plain shell or harness

Use this when the client is a CLI or harness with no native memory tools.

Install the wrappers from the repo:

```bash
./scripts/install-memory-attach-wrappers.sh
```

This installs:

- `modus-attach-carrier`
- `modus-codex`
- `modus-qwen`
- `modus-gemini`
- `modus-ollama`
- `modus-hermes`
- `modus-openclaw`
- `modus-opencode`

Examples:

```bash
modus-codex "Summarize the current task."
modus-attach-carrier qwen "Review this patch for regressions."
modus-openclaw "Reply with exactly: nominal."
```

## How To Use It

### Normal retrieval and storage

If your client is MCP-capable, ask it to:

- remember a durable preference
- recall prior project decisions
- search for related mission context
- capture a turn through `memory_capture` so Homing itself decides whether to store an episode, facts, both, or nothing

### Shell attachment

If your client is a plain shell, use the wrapper or raw attach command:

```bash
homing attach --carrier codex --prompt "Summarize the current task."
```

That run will recall hot memory first, augment the prompt, and write back receipts and traces.

### Governance

Use proposals when you want memory changes to be explicit and reviewable.

Examples:

```bash
homing propose-hot --fact-path memory/facts/general-preference.md --temperature hot --reason "Durable operator context."
homing propose-elder --fact-path memory/facts/founding-covenant.md --protection-class elder --reason "Long-horizon constitutional memory."
homing review-queue
homing resolve-review --status pending --set-status approved --reason "Approved after review."
```

## The Two Most Important Mental Models

First: not every useful agent is a true memory client.

That is why Homing supports both MCP and sovereign attachment.

Second: memory is not just storage.

It is routing, evidence, temperature, protection, review, and recall behavior. If a system only stores text and retrieves it later, it is not yet doing the harder part.

## Current Naming Reality

The public name is **Homing by MODUS**.

The current primary binary is **`homing`**.

The compatibility alias is **`modus-memory`**.

We changed the install surface without severing existing integrations. New setups should prefer `homing`. Existing scripts and MCP configurations can continue to use `modus-memory` while they migrate on their own schedule.

## Related Docs

- [README.md](../../README.md)
- [cmd/modus-memory/README.md](../../cmd/modus-memory/README.md)
- [docs/reference/homing-main-merge-summary-2026-04.md](./homing-main-merge-summary-2026-04.md)
- [docs/research/modus-memory-partner-brief-2026-04-16.md](../research/modus-memory-partner-brief-2026-04-16.md)
