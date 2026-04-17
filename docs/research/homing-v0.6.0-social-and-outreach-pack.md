# Homing by MODUS v0.6.0 Social And Outreach Pack

This pack is the final public-facing copy set for the `v0.6.0` release.

It is meant to be directly usable for:

- X posts
- Reddit launch copy
- reply threads and FAQ answers
- partner and harness outreach

## X Posts

### Post 1: Release Announcement

```text
Homing by MODUS v0.6.0 is live.

This is the release where the rename becomes operational:

- `homing` is now the primary binary
- `modus-memory` stays as a compatibility alias
- new `memory_capture` tool gives MCP clients a policy-driven write path
- standalone default vault now matches the docs: `~/vault`

Local-first, inspectable memory for agents.
Plain markdown. No hosted control plane.

https://github.com/GetModus/modus-memory/releases/tag/v0.6.0
```

### Post 2: Product Framing

```text
Most agent memory products still boil down to one of two things:

1. cloud continuity you rent from a provider
2. a pile of local notes with weak retrieval

Homing is trying to be a different category:

- local-first
- route-aware recall
- first-class episodes
- governed memory changes
- shell attachment as well as MCP

https://github.com/GetModus/modus-memory
```

### Post 3: Technical Angle

```text
One of the important changes in Homing v0.6.0:

MCP connectivity is no longer allowed to masquerade as automatic memory.

If a client wants durable writes, it should call `memory_capture`, `memory_store`, or `memory_episode_store`.

`memory_capture` is the clean lane:
- policy-driven
- supports `dry_run`
- can store an episode, facts, both, or nothing

https://github.com/GetModus/modus-memory/releases/tag/v0.6.0
```

### Post 4: Philosophy

```text
If your agent memory lives mainly in a provider cache, you do not own continuity.

You are renting it.

Homing is our local-first answer:
- plain markdown storage
- route-aware retrieval
- recall receipts
- governed review flows
- portability as a design principle, not an afterthought

https://github.com/GetModus/modus-memory
```

### Post 5: Animal Research Angle

```text
Some of the memory architecture in Homing came from animal-memory research:

- salmon -> route-aware homing
- food-caching birds -> episodic identity
- elephants -> protected long-horizon memory

The useful part is that it did not stay metaphorical.
It changed the actual design.

https://github.com/GetModus/modus-memory
```

## Reddit Launch Post

### Suggested Title

```text
We shipped Homing by MODUS v0.6.0: local-first, inspectable memory for agents
```

### Suggested Body

```text
We just shipped `v0.6.0` for Homing by MODUS.

This is the release where the rename becomes real in the shipped product surface, not just the docs.

What changed:

- `homing` is now the primary binary
- `modus-memory` stays as a compatibility alias
- new `memory_capture` tool gives MCP clients a policy-driven memory write path
- standalone default vault now matches the docs at `~/vault`
- release assets are now published under the `homing-*` names

The larger project direction is the same:

- local-first memory
- plain markdown storage
- route-aware retrieval
- first-class episodes
- durable recall receipts
- governed memory review
- shell-first attachment for carriers that are not tool-native

We care a lot about the distinction between:

1. true MCP clients that can actually call tools
2. shells and harnesses that need memory attached externally

That is why the project supports both direct MCP usage and sovereign attachment.

If you want the shortest description:

Homing is inspectable, local-first memory for agents that runs as a single binary and stores memory as plain markdown.

Release:
https://github.com/GetModus/modus-memory/releases/tag/v0.6.0

Repo:
https://github.com/GetModus/modus-memory
```

## Reddit FAQ Replies

### If someone asks “why not just use provider memory?”

```text
Because provider memory is convenient right up until continuity becomes strategically important.

Then the questions get sharper:

- who actually owns the memory
- can you inspect it
- can you back it up
- can you move it across clients
- can you tell what was actually recalled

Homing is built around owning that layer instead of renting it.
```

### If someone asks “what makes this different from a basic MCP memory server?”

```text
The short answer is that we are not trying to stop at search-and-store.

The runtime also includes route-aware retrieval, first-class episodes, recall receipts, governed memory review, shell attachment, portability auditing, readiness reporting, and policy-driven capture.

So the project is now closer to a memory subsystem than to a thin MCP example server.
```

### If someone asks “is the rename breaking old setups?”

```text
No. `homing` is now the primary binary, but `modus-memory` remains supported as a compatibility alias.

New setups should prefer `homing`.
Existing scripts and MCP configs can keep using `modus-memory` while they migrate.
```

## Partner Outreach One-Pager

### Short Intro

Homing by MODUS is a sovereign memory runtime for agents.

It gives an agent stack durable, local-first memory without outsourcing continuity to a provider-owned cache or control plane.

### What It Does

- stores memory as plain markdown instead of hiding state behind a hosted service
- supports both true MCP clients and plain shells / harnesses
- uses route-aware retrieval instead of one flat memory search
- supports first-class episodic identity with event and lineage fields
- leaves durable recall receipts so retrieval can be audited
- supports governed memory review instead of silent mutation

### Why It Matters For A Harness Or Agent Platform

Most harnesses still treat memory as either:

- provider-owned continuity, or
- a simple retrieval add-on

Homing is better suited to a platform that wants durable, inspectable, portable memory as infrastructure.

It can sit underneath:

- desktop MCP clients
- shell-based coding agents
- orchestrators
- higher-level agent products that want local memory without inventing their own subsystem from scratch

### Integration Story

There are two honest lanes:

- direct MCP for clients that can actually mount tools and call them
- sovereign attachment for shells and harnesses that cannot

That is important because not every useful agent runtime will ever be a true tool-native client.

### Release Facts

- primary binary: `homing`
- compatibility alias: `modus-memory`
- standalone default vault: `~/vault`
- storage: plain markdown
- published assets: macOS, Linux, Windows, plus checksums
- release: `v0.6.0`

### Close

If you want an agent memory layer that is local-first, inspectable, portable, and usable across both MCP clients and shell-native runtimes, Homing is ready to evaluate.
