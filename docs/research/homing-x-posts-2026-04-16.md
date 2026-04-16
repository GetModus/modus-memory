---
title: Homing X Posts
date: 2026-04-16
status: active
audience: public launch and product positioning
---

# Homing X Posts

This pack is organized by audience so we can post with different tones depending on whether we want to speak to builders, product people, or a broader audience interested in AI memory.

The public product name is **Homing by MODUS**.

The current command and binary name remain **`modus-memory`** for now.

## Post Set 1: Technical Builders

### 1. Rename + architecture

```text
We renamed our memory product to Homing by MODUS.

Why?

Because it stopped being “just a local memory server.”

It’s now a sovereign memory kernel for agents:
• route-aware retrieval
• episodic identity
• recall receipts
• governed memory review
• shell-first attachment

The binary is still `modus-memory` for now.
```

### 2. Flat retrieval is not enough

```text
A lot of AI memory systems still behave like flat retrieval over old context.

That’s not enough.

You need to know:
• what was stored
• why it exists
• how recall was routed
• what was actually consulted
• how stale memory is governed

That’s the level we care about with Homing.
```

### 3. MCP plus attachment

```text
One thing we care about a lot:

memory should work for both:
1. true MCP clients
2. plain shells and agent harnesses with no native memory tools

That’s why Homing has two lanes:
• direct MCP
• sovereign attachment

Not every useful agent runtime will ever be a tool-native client.
```

### 4. Evidence-bearing recall

```text
Retrieval should not be a polite hallucination.

In Homing, recall can leave durable proof:
• the query
• the harness
• the adapter
• the selected paths
• linked facts and episodes
• verification status when recall is critical

Memory should be inspectable, not mystical.
```

### 5. Sovereignty and portability

```text
If your agent memory only exists in a provider-owned cache, you do not own continuity.

You are renting it.

That’s why we keep pushing on:
• plain markdown storage
• local-first memory
• portability audits
• provider memory as subordinate, not sovereign

That is the philosophy behind Homing.
```

### 6. Why we did not rename the binary yet

```text
We rebranded the product to Homing by MODUS.
We did not rename the binary yet.

That split is intentional.

Brand should tell the story.
Commands should stay stable until migration is worth the churn.

So the public product is Homing.
The command is still `modus-memory`.
Truth first, aesthetics second.
```

## Post Set 2: Product And Platform Audience

### 7. The product line

```text
Homing by MODUS is what we call our memory layer for agents.

Not a chatbot history dump.
Not a cloud memory tax.

A local, inspectable, portable memory substrate that can serve both MCP clients and ordinary shell-based agents.

That is the product direction.
```

### 8. Why the name matters

```text
We changed the public name from a literal implementation label to Homing by MODUS.

Why?

Because strangers do not want “a memory server.”
They want an agent that gets back to the right context reliably.

Homing says the behavior.
The old name only said the plumbing.
```

### 9. The practical pain point

```text
Most AI tools still reset the relationship every time the session ends.

That means every new chat starts with:
• lost preferences
• lost decisions
• lost context
• lost continuity

Homing is our attempt to make continuity feel owned, inspectable, and portable instead of rented and fragile.
```

### 10. The differentiator

```text
The interesting question is not “does your agent have memory?”

The interesting questions are:
• who owns it
• can you inspect it
• can it survive model swaps
• can shells use it too
• can it prove what it recalled

That is where we’re trying to be opinionated with Homing.
```

### 11. The local-first case

```text
Cloud memory is convenient right up until it becomes strategic.

Then the questions get awkward:
• where does user continuity actually live
• how portable is it
• what happens if the provider changes direction
• what can the user inspect or export

Homing is our local-first answer to those questions.
```

## Post Set 3: Research And Design Angle

### 12. Animal research summary

```text
The recent memory redesign was shaped by animal-memory research more than most people would expect.

Salmon influenced route-aware retrieval.
Food-caching birds influenced episodic identity.
Elephants influenced protected elder memory.

The fun part is that none of this stayed metaphorical. It changed the actual architecture.
```

### 13. Salmon

```text
Salmon gave us one of the most useful memory ideas:

don’t treat retrieval like one flat search.

Home in stages.

Coarse route first.
Local cue second.
Final ranking last.

That idea maps surprisingly well to agent memory, and it’s now part of how we think about recall in Homing.
```

### 14. Food-caching birds

```text
Food-caching birds helped sharpen a problem in AI memory:

similar events should not collapse into one semantic blur.

That pushed us toward:
• episodes
• event IDs
• lineage IDs
• content hashes
• cue-bearing traces

Memory needs identity, not just similarity.
```

### 15. Elephants

```text
Elephants remind you that old memory is not the same thing as irrelevant memory.

Some knowledge is rare, old, and still critical.

That pushed us toward explicit elder-memory protection so long-horizon lessons and commitments do not get buried by recency bias.
```

### 16. The higher-level takeaway

```text
The best thing the animal-memory research gave us was not branding.

It gave us design constraints:
• route before ranking
• identity before abstraction
• protection before decay

That is a much better starting point for AI memory than “just store more context.”
```

## Post Set 4: Broader Audience

### 17. Human-readable version

```text
We renamed our memory product to Homing.

Because good AI memory should feel less like “saving chat history” and more like helping an agent find its way back to what matters.

That is the idea behind the new name.
```

### 18. One-line positioning

```text
Homing by MODUS:
a local memory layer for agents that keeps continuity on your machine instead of renting it from the cloud.
```

### 19. Short launch note

```text
We just pushed the first public branding pass for Homing by MODUS.

Same underlying system.
Better public name.
Clearer story.

The command is still `modus-memory` for now, but the product direction is now much easier to explain.
```

### 20. Opinionated close

```text
“AI memory” should not mean:
“we kept your old chats somewhere.”

It should mean:
you can inspect it
you can move it
you can trust its lineage
it can survive model swaps

That’s the bar we’re aiming at with Homing.
```

## Suggested Posting Flow

If we want a clean sequence instead of a random burst, this order is strong:

1. Post 19 for the basic update
2. Post 8 for the rename rationale
3. Post 12 for the research hook
4. Post 13 or 14 for a more technical follow-up
5. Post 10 for the product differentiation angle

## Short Replies If People Ask “Why Homing?”

```text
Because the system is about getting an agent back to the right context, not just storing old text.
```

```text
Because route-aware memory is closer to homing than to generic search.
```

```text
Because the product outgrew the old implementation-shaped name.
```

## Short Replies If People Ask “Did You Rename The Binary?”

```text
Not yet. The public product is Homing by MODUS. The command is still `modus-memory` while release plumbing catches up.
```

```text
We changed the story first and kept the install surface stable. That felt like the honest order.
```
