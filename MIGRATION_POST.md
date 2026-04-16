# Khoj cloud is shutting down April 15 — here's a zero-setup alternative

Khoj announced their cloud service is shutting down. If you've been using it for AI memory, you need to move your data somewhere.

I built [modus-memory](https://github.com/GetModus/modus-memory), now presented publicly as **Homing by MODUS**, as a local-first memory runtime. One binary, no required database, no Docker, no Python environment. Your data stays as plain markdown files on your disk.

## Migration takes 5 minutes

```bash
# Install
go install github.com/GetModus/modus-memory@latest

# Export from Khoj (Settings → Export → save ZIP)
# Import
modus-memory import khoj ~/Downloads/khoj-conversations.zip

# Validate
modus-memory doctor
```

That's it. Your conversations become searchable documents. Context references become memory facts. Entities are auto-extracted into a knowledge graph. Everything is plain `.md` files you can read, edit, grep, and back up with git.

## What it does

- **BM25 full-text search** with field boosting — searches 19,000 documents in <5ms
- **FSRS spaced repetition** — old memories naturally fade, important ones strengthen on recall
- **Cross-referencing** — facts linked by subject, tag, and entity
- **MCP protocol** — works with Claude Desktop, Cursor, Windsurf, any MCP client
- **~8MB stripped binary** — compact enough to download and run quickly

## What it doesn't do

- No cloud. Your data never leaves your machine.
- No API keys needed for search (no LLM calls on the hot path).
- No Docker, no PostgreSQL, no Python.

## Comparison

| | Khoj (self-hosted) | modus-memory |
|---|---|---|
| Setup | Python + Docker + PostgreSQL + embeddings | One binary |
| Storage | PostgreSQL | Plain markdown files |
| Search | Embeddings (needs GPU/API) | BM25 (instant, no GPU) |
| Memory decay | No | FSRS spaced repetition |
| Size | ~2GB+ with deps | ~8MB stripped |
| Data portability | Database export | Files on disk |

## Post-import health check

After importing, `modus-memory doctor` validates your vault:

```
Documents: 847
Facts: 234 total, 234 active
Cross-refs: 89 subjects, 45 tags, 67 entities

─── Diagnostics ───
[WARN] 3 duplicate subject+predicate pairs
[INFO] 12 documents with empty body

─── Distribution ───
  brain/               423 docs
  memory/              234 docs
  atlas/               190 docs

1 warnings, 1 info. Run after cleanup to verify.
```

Completely free. MIT licensed. [GitHub](https://github.com/GetModus/modus-memory).

---

*Built this because I needed AI memory that I actually own. Happy to answer questions.*
