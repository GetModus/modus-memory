---
title: Homing by MODUS Partner Brief
date: 2026-04-16
audience: harness and agent platform partners
status: active
scope: recent memory evolution across the April 14-15 Grade S campaign and the current branch state
---

# Homing by MODUS Partner Brief

## Executive Summary

Over the last several sessions, MODUS Memory has stopped being "persistent notes for an agent" and has become a sovereign memory substrate with explicit memory law, provenance, episodic identity, route-aware retrieval, governed promotion and demotion, replay-driven consolidation, secure-state auditing, portability auditing, and direct attachment lanes for shell-native agents that do not expose memory tools of their own.

The important change is architectural, not cosmetic. MODUS now treats memory as a harness function and a local kernel. MCP is one adapter. The TUI is one adapter. The operator shell is one adapter. A plain carrier such as Codex, Qwen, Gemini, OpenClaw, Hermes, Ollama, or OpenCode can also be run through the same memory authority by sovereign attachment wrappers, with recall receipts, traces, and optional episode capture written back into the vault.

This matters for harness and agent platforms because the system is no longer merely a retrieval server. It is becoming a full memory control plane: local-first, plain-markdown, reviewable, portable, measurable, and designed to survive provider churn rather than depend on it.

## The Short Version Of What Changed

Before this campaign, MODUS already had durable facts, raw vault material, hybrid retrieval, and a strategic commitment to memory sovereignty. During the recent sessions, that foundation was turned into a much stricter and more ambitious system.

Memory now has hot versus warm admission law, explicit hot-tier governance, first-class episodic objects, recall receipts, a direct memory kernel, route-aware retrieval selectors, source verification, temporal truth fields, structural linking, elder-memory protection logic, replay promotion logic, secure-state manifests, portability audits against provider-side caches, readiness reports, authored live trials, synthetic evaluation suites, carrier audits, carrier probes, and shell-first governance commands.

In plainer terms, memory is no longer just stored. It is admitted, routed, cited, verified, reviewed, linked, protected, exercised, and audited.

## Why We Built It This Way

Three convictions drove the recent work.

First, sovereign memory is a strategic necessity. Memory that lives primarily in Claude, Codex, or any other provider cache is not truly ours. It is rented continuity. The mission file for Memory Sovereignty states the north star plainly: the LLM is the carrier, not the store.

Second, raw and derived memory must coexist. Raw material preserves what actually happened. Derived memory makes that usable. If we keep only raw transcripts, the system is inert. If we keep only abstractions, the system drifts and loses provenance.

Third, memory must be governed like system state rather than treated like a pile of notes. That is why recent work kept insisting on explicit artifacts, explicit review, append-only evidence, and bounded automatic admission instead of quiet mutation.

## The Key Inspirations

The design was shaped by three classes of influence at once.

Human-memory research supplied the lifecycle model. Craik and Tulving reinforced deep encoding, Tulving and Thomson supplied encoding specificity, Roediger and Karpicke reinforced retrieval practice, Cepeda added spacing logic, and Hupbach added reconsolidation and lawful updating on recall.

Exceptional biological memory supplied the architecture metaphors. Food-caching birds supplied the idea of sparse episodic barcodes. Salmon supplied hierarchical homing from coarse route to local cue. Elephants supplied protected elder knowledge and the need to keep rare but consequential old memory from being buried by freshness bias.

Security doctrine supplied the hardening model. Apple Platform Security, NIST key-management and log-management ideas, and Saltzer and Schroeder pushed memory toward mediated access, protection classes, append-only audit, tamper evidence, and rollback awareness.

The internal April 11 memory review then translated public industry thinking into MODUS-native doctrine. The useful takeaways were that memory is inseparable from the harness, all designs are trade-offs across stable axes, and open memory ownership is a moat while provider-owned harness memory is lock-in.

## The Chronology

### 1. Foundation Before The Grade S Journal

Three lawful tranches were already in place before the dedicated Grade S file was opened.

- Tranche One added provenance-bearing durable fact fields. Facts began carrying `created_at`, `memory_temperature`, `source`, `source_ref`, `source_lineage`, `captured_by_office`, and `captured_by_subsystem`, and retrieval began reranking by evidence rather than lexical convenience alone.
- Tranche Two enforced hot-versus-warm admission law. Only hot facts became eligible for automatic session-prep and per-turn admission. Warm memory stayed durable and retrievable, but not ambient.
- Tranche Three made the hot tier governed rather than merely labeled. Hot promotion and demotion began flowing through explicit review artifacts in `memory/maintenance`, with a cap, stale-review thresholds, and apply-time proof written to the operations ledger.

This foundation is recorded in `vault/sessions/2026-04-14-modus-operator-shell-and-memory-hardening.md`.

### 2. April 14: The Grade S Memory Program Formalized The Ambition

The Grade S campaign file changed the level of ambition. Memory was no longer framed as a nicer memory tool. It was framed as "a memory organism": sovereign, provenance-rich, episodic, replay-capable, hierarchically navigable, bounded on ordinary hardware, and capable of strengthening, cooling, reviewing, and protecting its own state without provider dependence.

That file also established the active build order after the foundation:

1. episodic identity
2. recall receipts and retrieval-as-write
3. hierarchical navigation
4. replay and consolidation
5. elder memory
6. secure-state boundary
7. evaluation harness

### 3. April 14: Phase 1 Added Episodic Identity

This was the first major change in the current campaign. `internal/vault/episodes.go` introduced first-class episodic objects under `vault/memory/episodes/`. These episodes carry:

- `event_id`
- `lineage_id`
- `content_hash`
- `event_kind`
- `cue_terms`
- provenance-bearing write authority
- related facts, episodes, entities, and missions

The conceptual purpose was to stop collapsing semantically similar experiences into one another. An episode is the barcode-like identity surface behind later semantic memory.

This phase also extended facts so semantic memory can point back to episodes through `source_event_id`, `lineage_id`, and cue terms.

### 4. April 14: Phase 2 Inverted The Architecture Into A Memory Kernel

`internal/memorykit/kernel.go` turned memory into a direct attachment contract instead of another transport-owned tool surface.

The kernel now owns:

- episode writes
- fact writes
- fact retrieval
- hot-context composition

MCP, the agent registry, and the TUI are treated as adapters around that kernel rather than as independent owners of memory law.

This is the moment the project became plausibly productizable as a harness-grade memory layer instead of a server-shaped curiosity.

### 5. April 14: Phase 3 Made Retrieval Durable Through Recall Receipts

`internal/vault/recalls.go` added first-class recall receipts under `vault/memory/recalls/`.

A recall now records:

- `recall_id`
- query
- harness
- adapter
- mode
- temperature filter
- selected paths
- source-event echoes
- lineage echoes
- cue terms
- linked facts, episodes, entities, and missions
- optional verification results
- optional work-item linkage

Retrieval is therefore no longer a transient courtesy. It is a durable act that can be audited, reinforced, and later inspected. Successful recall also reinforces facts through FSRS-style strengthening.

### 6. April 15 And The Current Tree: The System Expanded Beyond The Journal

The April 15 AI-memory scan provided external justification for the next set of features, but the current worktree and live reports show that several of those next faculties are already materially present.

The memory estate now includes the following layers beyond the first three Grade S phases.

## What MODUS Memory Has Become

### A. A Sovereign Local Memory Kernel

The kernel in `internal/memorykit/kernel.go` is the center of gravity. It gives MODUS a single memory authority that can serve MCP clients, shell wrappers, internal tools, the TUI, and the operator shell.

This is the decisive shift for partner conversations. The product is not "an MCP memory server." It is "a memory kernel with adapters."

### B. A Dual-Layer Durable Store: Facts Plus Episodes

The system now maintains both:

- semantic facts under `vault/memory/facts/`
- episodic traces under `vault/memory/episodes/`

Facts are derived and operational. Episodes are raw or near-raw and identity-bearing. This lets the system keep both human-usable memory and evidence-bearing lineage.

### C. Admission Discipline: Hot And Warm Memory

Automatic memory injection is now commissioned, not improvised.

Hot facts:

- are the only facts eligible for automatic session-start and per-turn admission
- are bounded by a shared cap
- require explicit review artifacts for transition

Warm facts:

- remain searchable and durable
- do not automatically pollute context

This answers one of the central failure modes of agent memory systems: stale context dominance.

### D. Governed Hot-Tier Review

`internal/maintain/hot.go` and `internal/maintain/apply.go` now create and apply explicit hot-memory transition candidates.

The system supports:

- warm-to-hot proposals
- hot-to-warm proposals
- overflow review when the hot tier exceeds cap
- stale hot review after an age threshold
- fact-level transition history
- ledger proof of review generation and approved application

Nothing rewrites itself silently. Review artifacts mediate change.

### E. Hierarchical And Route-Aware Retrieval

This is one of the most important recent upgrades and one of the easiest to miss if one only reads the mission prose.

`internal/vault/facts.go` now supports route-aware retrieval selectors including:

- `RouteSubject`
- `RouteMission`
- `CapturedByOffice`
- `CueTerms`
- `TimeBand`
- `WorkItemID`
- `LineageID`
- `Environment`

The retrieval path no longer behaves like one flat semantic heap. It can narrow by mission, subject, office, time, work item, lineage, and environment before final ranking. That is the beginning of salmon-style homing in software form.

### F. Provenance-Aware Ranking And Structured Fact Facets

Fact ranking now considers more than lexical match. It also weighs:

- confidence
- importance
- provenance completeness
- protection class
- temperature
- freshness
- temporal status

Rendered recall lines also expose useful facets such as:

- `superseded`
- `expired`
- `elder`
- source identity
- validity windows
- counts of linked facts, episodes, entities, and missions

This makes recall more inspectable and less magical.

### G. Temporal Truth Rather Than Bare Creation Time

Facts are no longer limited to `created_at`. The current model supports:

- `observed_at`
- `valid_from`
- `valid_to`
- `temporal_status`
- supersession relationships

`internal/maintain/temporal.go` creates explicit `candidate_fact_temporal_transition` artifacts so facts can be marked active, expired, or superseded under review rather than via casual overwrite.

This is a direct answer to the April 15 conclusion that memory systems need time semantics, not just similarity retrieval.

### H. Source Verification For Critical Recall

`internal/vault/verification.go` adds fact verification for critical or high-stakes recall.

On a critical retrieval path, the system can annotate facts as:

- `verified`
- `review_required`
- `mismatch`
- `unverified`
- `source_missing`

Verification reopens the cited source material, checks whether the source text directly supports the stored claim, and reflects that result into the recall receipt itself.

This matters for any partner claiming agent trustworthiness. It is the difference between "the memory said so" and "the system reopened the underlying evidence and marked whether the claim was actually supported."

### I. Structural Linking Across Facts, Episodes, Entities, And Missions

`internal/maintain/structural.go` backfills explicit structure around facts.

The system can propose additive links based on shared:

- `source_event_id`
- `lineage_id`
- `work_item_id`
- mission-and-subject combinations
- exact atlas entity matches

Resulting facts can carry:

- `related_fact_paths`
- `related_episode_paths`
- `related_entity_refs`
- `related_mission_refs`

This is the beginning of graph-like memory without requiring a heavyweight graph database.

### J. Elder Memory As Protected Long-Horizon Knowledge

`internal/maintain/elder.go` introduces a protected posture for rare, high-consequence memory.

The elder tier supports:

- promotion candidates for memories that should not be buried by recency
- overflow review when protected memory exceeds cap
- stale elder anomaly review
- contradiction anomaly review when protected elder memory is implicated in conflict artifacts

This is the elephant-memory idea rendered as governance law: old, important memory deserves explicit protection from ordinary recency bias.

### K. Replay-Driven Promotion

`internal/maintain/replay.go` scans episodes and recall receipts for repeated evidence and can emit `candidate_replay_fact` artifacts.

Replay promotion looks for:

- repeated episode evidence
- supporting recall history
- consensus on source event or lineage when possible

It then proposes semantic promotion rather than mutating canonical facts directly.

This is how the system moves from "stored episodes" toward "earned semantic memory."

### L. Corrections That Propagate Without Silent Rewrite

`internal/vault/corrections.go` stores corrections under `memory/corrections/` and flags affected facts, recalls, and maintenance artifacts for review.

Key behavior:

- corrections do not rewrite canonical facts silently
- affected documents get `correction_review_status: pending`
- affected artifacts are gathered into a `candidate_correction_propagation` review artifact

This gives the estate a lawful way to respond to discovered error without pretending that corrections happen outside history.

### M. Secure-State Manifests And Rollback Detection

`internal/memorykit/secure_state.go` is one of the clearest signs that the project has moved beyond ordinary agent memory.

The system now writes a secure-state manifest across:

- `memory/facts`
- `memory/episodes`
- `memory/recalls`
- `memory/maintenance`

That manifest tracks:

- generation number
- root hash
- previous root hash
- file count
- class counts
- per-file content hashes
- security classes
- signature and ledger proof

Verification can then detect:

- post-manifest drift
- rollback suspicion when the ledger already knows a newer root hash than the manifest on disk

There is also a deliberate error boundary: sealed memory is recognized doctrinally, but plaintext vault storage does not pretend to implement sealed payload protection yet. The code refuses to lie about that.

### N. Portability Audits Against Provider-Side Residue

`internal/memorykit/portability.go` audits external cache memory, especially Claude-side project memory, and scores how much of it has been mirrored or superseded by sovereign memory.

Coverage classes include:

- `explicit_runtime_equivalent`
- `exact_vault_counterpart`
- `cited_into_sovereign_memory`
- `external_only`

This turns "memory sovereignty" from a slogan into a measurable portability surface.

The system can also:

- build a portability queue
- archive external-only residue into the vault
- write portability reports to `state/memory/portability/`

### O. Direct Sovereign Attachment For Shell Carriers

`internal/memorykit/attachment.go` and the wrapper scripts are the partner-facing story in concentrated form.

MODUS can now run shell-native carriers through a sovereign attachment lane that does the following:

1. recall hot memory
2. augment the prompt
3. execute the carrier
4. write a recall receipt
5. write a trace
6. optionally write an episode

Supported carrier runners in the current tree include:

- Codex
- Claude
- Qwen
- Gemini
- Hermes
- OpenClaw
- Ollama
- OpenCode

This means a harness does not need to become a native MCP memory client in order to inherit memory law. It can simply call a stable attachment command.

### P. Shell-First Governance Commands

The `modus-memory` command now exposes memory governance as first-class CLI behavior.

Notable commands include:

- `attach`
- `propose-hot`
- `propose-structural`
- `propose-temporal`
- `propose-elder`
- `review-queue`
- `resolve-review`
- `carrier-audit`
- `carrier-probe`

This matters operationally because it turns memory stewardship into a composable shell surface rather than a GUI-only ritual or a manual markdown chore.

### Q. Carrier Audits And Live Probes

`internal/memorykit/carriers.go` and `carrier_probe.go` inspect whether the local carrier estate is actually ready and can perform live sovereign-attachment probes against selected carriers.

This gives the system two valuable properties:

- doctrinal readiness without live execution
- live attachment proof when requested

That is useful for both internal operations and external demonstrations.

### R. Trials, Synthetic Evaluation, And Readiness

The system now grades itself through three distinct report surfaces.

`internal/memorykit/trials.go` runs authored live-vault trial cases against the present estate.

`internal/memorykit/eval.go` runs a synthetic fixture suite across the live code paths for:

- interference recall precision
- elder retention
- replay promotion accuracy
- hot-tier stale detection
- secure-state tamper detection
- secure-state rollback detection

`internal/memorykit/readiness.go` combines shelf counts, pending maintenance, trial score, evaluation score, portability score, and secure-state verification into a single readiness judgment.

This is the point at which memory ceases to be admired from prose and starts being scored like a subsystem.

### S. Operator Surfaces And Integration

In the standalone repo, Homing now exposes the memory kernel, governance flows, and sovereign attachment lane directly through `cmd/modus-memory`.

In the broader MODUS OS estate, those same memory surfaces can be mounted into richer operator interfaces, but the important product truth for partners is that the standalone repository already contains the governed memory runtime itself.

## Live Proof As Of The Latest Reports

The strongest way to describe what MODUS Memory has become is to cite the system's own generated reports.

The latest memory readiness report at `vault/state/memory/readiness/latest.md`, generated on `2026-04-15T23:10:47Z`, reports:

- status: `ready_for_pretesting`
- facts: `7653`
- hot facts: `5`
- warm facts: `14`
- structured facts: `3`
- episodes: `21`
- recall receipts: `200`
- maintenance artifacts: `457`
- pending maintenance artifacts: `0`
- live trial score: `1.00` (`4/4`)
- synthetic evaluation score: `1.00` (`6/6`)
- portability score: `1.00`
- secure-state verified: `true`

The filesystem snapshot taken while drafting this brief on `2026-04-16` shows the estate has already advanced slightly further to:

- `7653` fact files
- `22` episode files
- `202` recall receipts
- `458` maintenance artifacts

The latest live trial report at `vault/state/memory/trials/latest.md` shows all four authored sovereign-vault trials passing, including:

- subject-routed retrieval for the General's iMessage preference
- general preference retrieval with linked structure
- critical recall source-warning behavior on an older uncited fact
- mission context retrieval for the three pillars of the mission

The latest synthetic evaluation report at `vault/state/memory/evaluations/latest.md` shows all six synthetic cases passing, including secure-state tamper and rollback detection.

The latest portability audit at `vault/state/memory/portability/latest.md` reports:

- inspected external cache files: `91`
- covered by sovereign memory surfaces: `91`
- external-only residue: `0`
- coverage score: `1.00`

The latest carrier audit at `vault/state/memory/carriers/latest.md` reports:

- total carriers inspected: `8`
- ready: `7`
- dormant by doctrine: `1`
- missing: `0`

The latest carrier probe at `vault/state/memory/carriers/probes/latest.md` shows a successful live sovereign-attachment probe against Codex, including a recall receipt, trace, thread ID, and output preview of `nominal`.

## Why This Is Distinctive For Harness And Agent Platforms

Most agent-memory products sit in one of three buckets.

They are either a minimal MCP memory server, a hosted SaaS memory layer, or a retrieval service with pleasant rhetoric around "long-term memory" but little explicit governance.

MODUS Memory is becoming something else.

It is:

- local-first and plain-markdown by default
- designed as a harness function rather than a plugin afterthought
- adapter-friendly across MCP, shell wrappers, internal dashboards, and future runtimes
- provenance-bearing down to facts, episodes, recalls, and review artifacts
- explicit about promotion, correction, supersession, and protection
- measurable through trials, evaluation, readiness, portability, and secure-state verification
- honest about what is not yet fully sealed instead of bluffing security theater

That combination is unusual. It makes the system interesting not only as a personal memory tool, but as a candidate memory layer for agent harnesses that want to advertise real continuity without surrendering user state to provider clouds.

## The Product Story In One Sentence

MODUS Memory has become a sovereign memory kernel for agents: a local, plain-text, reviewable, route-aware, replay-capable, security-conscious memory substrate that can attach to both true MCP clients and ordinary shell carriers while preserving durable proof of what was stored, recalled, linked, reviewed, and trusted.

## What Still Remains To Be Finished

The system is much stronger than it was a few sessions ago, but the estate is not complete and should not be presented as finished divinity.

The major remaining edges are:

- sealed memory is acknowledged doctrinally but not yet implemented as protected payload storage inside the plaintext vault
- the elder tier exists in code and evaluation, but the live readiness report still shows `0` elder-protected facts at the moment of that report
- structural linking is present, but the live readiness report shows only `3` structured facts and `0` structured episodes, so the structure layer is still early
- the replay path is real and tested, but it remains proposal-oriented rather than autonomous canon settlement
- the system is ready for pretesting, not yet declared production-hardened across every carrier and every operating environment

These are honorable limitations. They make the brief credible.

## Suggested External Positioning

If this is shown to a potential harness or agent partner, the cleanest positioning is not "we built memory." Everyone says that.

The cleaner claim is that MODUS is building a memory kernel for agents that:

- keeps memory sovereign and portable
- distinguishes raw episodes from semantic facts
- treats retrieval as an auditable state transition
- uses governance artifacts instead of silent mutation
- supports direct attachment for shell-native carriers
- measures itself through trials, evaluation, portability, and secure-state verification

That is a much more serious sentence, and it happens to be true.

## Source Spine For This Brief

The brief is based on the following recent campaign and implementation sources:

- `vault/missions/active/memory-sovereignty--migrate-claude-memory--modus-archive.md`
- `vault/missions/active/grade-s-memory-program-0f6c7d.md`
- `vault/sessions/2026-04-14-modus-operator-shell-and-memory-hardening.md`
- `vault/sessions/2026-04-14-grade-s-memory-program.md`
- `vault/sessions/2026-04-15-ai-memory-advancements-scan.md`
- `vault/modus/memory-architecture-doctrine.md`
- `vault/modus/memory-grade-s-implementation-note.md`
- `/Users/modus/Desktop/session-memory-architecture-review.md`
- `cmd/modus-memory/README.md`
- `cmd/modus-memory/main.go`
- `internal/memorykit/*.go`
- `internal/memorycli/*.go`
- `internal/maintain/*.go`
- `internal/vault/episodes.go`
- `internal/vault/recalls.go`
- `internal/vault/verification.go`
- `internal/vault/corrections.go`
- `internal/vault/facts.go`
- `vault/state/memory/readiness/latest.md`
- `vault/state/memory/trials/latest.md`
- `vault/state/memory/evaluations/latest.md`
- `vault/state/memory/portability/latest.md`
- `vault/state/memory/carriers/latest.md`
- `vault/state/memory/carriers/probes/latest.md`

## Final Judgment

Across the last several sessions, MODUS Memory has evolved from a sovereignty project into a partner-worthy memory architecture.

It now has doctrine, kernel boundaries, multiple evidence-bearing memory forms, retrieval receipts, governance artifacts, shell attachment, portability auditing, secure-state manifests, and self-scoring proof surfaces. In other words, it has become a serious answer to the question of what memory for agents should look like when one intends to own it rather than rent it.
