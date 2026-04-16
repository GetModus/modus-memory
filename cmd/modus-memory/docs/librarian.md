# The Librarian Pattern

A local LLM that serves as the sole gatekeeper for your memory vault.

## The Problem

Most AI memory systems let any model read and write freely. This creates predictable problems:

- **Context bloat** — the cloud model sees everything, including noise, duplicates, and stale facts
- **Inconsistent storage** — different models tag, classify, and format memories differently
- **Token waste** — expensive frontier models spend tokens on retrieval, filtering, and filing instead of reasoning
- **No sovereignty** — your memories scatter across whatever model happens to be active

## The Solution

Run a small, dedicated local model — the **Librarian** — whose only job is memory stewardship. It sits between your vault and whatever model is doing the actual reasoning:

```
┌─────────────┐     ┌────────────────┐     ┌──────────────┐
│ Cloud Model  │◄───►│   Librarian    │◄───►│ modus-memory │
│ (reasoning)  │     │ (local, ~8B)   │     │   (vault)    │
└─────────────┘     └────────────────┘     └──────────────┘
                     Sole write access
                     Query expansion
                     Relevance filtering
                     Context compression
```

The cloud model stays focused on reasoning. The Librarian handles the boring-but-critical infrastructure: retrieval, filing, deduplication, decay, and context curation. It hands the cloud model only the 4-8k tokens that actually matter.

## Why This Works

**Token and cost discipline.** Cloud models are expensive once context balloons. The Librarian aggressively prunes, reranks, and synthesizes memories (using BM25 + FSRS + cross-references) before anything touches the cloud. You're running a local "memory compiler" so the expensive model only sees high-signal context.

**Context hygiene.** Cloud models get distracted by noise. The Librarian can pull the last 3 conversations about a topic, expand technical terms, drop anything older than 30 days unless FSRS says it's still relevant, and format it as a clean prompt. The cloud model stays focused.

**Privacy and sovereignty.** Sensitive data never leaves your machine unless you explicitly want it to. The Librarian can decide "this memory has PII — keep it local-only."

**Speed.** Local model runs on whatever hardware you have. No network round-trips for memory lookups. Searches return in microseconds.

## The System Prompt

This is the core prompt that defines the Librarian role. Adapt it to your model and use case:

```markdown
You are the Librarian — the sole steward of persistent memory and vault state.

You are the ONLY model authorized to write to the vault. All other models
delegate storage operations through you. When another model needs something
stored, corrected, or retrieved, it comes through you.

## Write Sovereignty

You are the single point of truth for all persistent state:
- memory_store: Store new facts (subject/predicate/value)
- vault_write: Write or update vault documents
- vault_search/vault_read: Retrieve information on behalf of other models

No other model writes to the vault. Models may change. The sovereignty
of this office does not.

## Operations

Filing: When another model hands you content (research, analysis, code docs),
file it in the correct vault location with proper frontmatter, tags, and
importance level.

Retrieval: When the reasoning model needs context, search the vault and
return only relevant, high-signal results. Compress and deduplicate before
returning. The reasoning model should never see noise.

Triage: Classify incoming items:
- ADAPT — valuable, add to vault with proper metadata
- KEEP — store as-is for reference
- DISCARD — not worth storing

Use importance levels: critical, high, medium, low.

Maintenance: Check for existing entries before creating new ones. Merge
related facts. Flag stale entries for decay. Reinforce facts that keep
getting accessed.

## Filing Rules

- Every vault entry needs YAML frontmatter (title, type, tags, status)
- Memory facts need subject, predicate, value, importance, and confidence
- Update rather than duplicate — always check first
- Tag everything — tags are how facts get found later
- Be concise and orderly. You are a custodian of records.
```

## Recommended Models

The Librarian doesn't need a frontier model. It needs a fast, reliable model that follows instructions precisely. Good candidates:

| Model | Size | Notes |
|-------|------|-------|
| Gemma 4 | 12B-27B | Strong instruction following, multimodal |
| Qwen 3 | 8B-14B | Good structured output, fast |
| Llama 3 | 8B | Solid baseline, widely available |
| Phi-4 | 14B | Compact, good at structured tasks |

We run ours on Gemma 4 27B via Ollama. Any model that can reliably produce YAML frontmatter and follow classification rules will work.

## Connecting to modus-memory

### Option 1: MCP Tool Delegation

If your AI client supports MCP, the simplest setup is tool-level delegation. Configure your client so the Librarian model handles memory tool calls:

```json
{
  "mcpServers": {
    "memory": {
      "command": "modus-memory",
      "args": ["--vault", "~/vault"]
    }
  }
}
```

Then instruct your reasoning model: "For any memory operation, delegate to the Librarian. Do not write to memory directly."

### Option 2: Prompt Pipeline

For tighter control, run the Librarian as a preprocessing step:

```bash
# 1. Librarian retrieves relevant context
echo "User is asking about React authentication patterns" | \
  curl -s http://127.0.0.1:8090/v1/chat/completions \
    -H "Content-Type: application/json" \
    -d @- <<'JSON'
{
  "model": "mlx-community/gemma-4-26b-a4b-it-4bit",
  "messages": [
    {"role": "system", "content": "You are the Librarian. Search the vault for relevant context and return a compressed summary."},
    {"role": "user", "content": "User is asking about React authentication patterns"}
  ],
  "max_tokens": 512,
  "temperature": 0.2
}
JSON

# 2. Feed the Librarian's output to the cloud model as system context
```

### Option 3: Agent Framework

In a multi-agent setup, the Librarian is one agent in the fleet with exclusive write access to vault tools. Other agents (coder, researcher, reasoner) request storage through the Librarian agent.

```yaml
# Agent role definition
name: librarian
model: mlx-community/gemma-4-26b-a4b-it-4bit
tools: [vault_search, vault_read, vault_write, vault_list, memory_store, memory_search]
```

## Example: Retrieval Flow

**User asks the cloud model:** "What did we decide about the auth refactor?"

**Cloud model delegates to Librarian:** "Search vault for decisions about auth refactor"

**Librarian runs:**
1. `vault_search("auth refactor decision")` — BM25 with query expansion finds "authentication," "OAuth," "session tokens"
2. Gets 12 results, filters by FSRS retrievability (drops 4 stale entries)
3. Deduplicates (2 entries say the same thing)
4. Returns compressed context: 6 relevant facts, ~800 tokens

**Cloud model receives:** Clean, high-signal context. Reasons over it. Responds to user.

**Total cloud tokens spent on memory:** ~800 instead of ~8,000.

## Example: Storage Flow

**Cloud model produces analysis:** 2,000 words on microservice trade-offs.

**Cloud model delegates to Librarian:** "File this analysis in the vault"

**Librarian runs:**
1. Checks for existing entries on "microservice architecture" — finds 2
2. Merges new insights with existing entries rather than duplicating
3. Writes updated document with proper frontmatter, tags, importance: high
4. Stores 3 new memory facts (subject/predicate/value) for quick retrieval
5. Confirms: "Filed. Updated 1 document, added 3 facts."

**Result:** One clean, deduplicated entry instead of scattered fragments.

## Design Principles

1. **Single writer.** One model owns all writes. No conflicts, no inconsistency.
2. **Small model, focused job.** The Librarian doesn't need to be smart. It needs to be reliable.
3. **Compress before crossing the boundary.** Every token that crosses from local to cloud costs money. Minimize.
4. **Plain files as ground truth.** Markdown + YAML frontmatter. Readable, greppable, git-friendly. No proprietary format.
5. **Decay is a feature.** FSRS ensures old noise fades while important memories strengthen. The Librarian doesn't hoard — it curates.

---

The Librarian pattern turns modus-memory from a storage layer into a complete personal memory system. The vault holds the data. The Librarian decides what goes in, what comes out, and what fades away.

Your memory. Your machine. Your archivist.
