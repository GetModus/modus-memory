# Changelog

## v0.5.0 — Homing by MODUS

`v0.5.0` is the first release in the **Homing by MODUS** line.

This release turns `modus-memory` from a lightweight local memory server into a more complete sovereign memory runtime for agents. The public product is now **Homing by MODUS**. The binary, package, and module name remain `modus-memory` for compatibility.

### Added

- Route-aware retrieval across subject, mission, office, work item, lineage, environment, time band, and cue terms
- First-class episodic memory objects with event identity and lineage
- Durable recall receipts and procedural traces
- Governed memory review flows for hot, temporal, elder, structural, and replay-driven changes
- Shell-first sovereign attachment for carriers such as Codex, Qwen, Gemini, Ollama, Hermes, OpenClaw, and OpenCode
- Secure-state verification, portability auditing, readiness reporting, synthetic evaluation, and live trial runs
- Partner-facing documentation, release notes, and launch materials

### Changed

- Product branding now presents the system as **Homing by MODUS**
- README and product docs now describe the runtime as a sovereign memory kernel rather than only an MCP memory server
- The standalone MCP surface now includes 22 core tools and 5 Pro extensions
- Binary version advanced from the pre-Homing line to `0.5.0`

### Compatibility

- Command remains `modus-memory`
- Module path remains `github.com/GetModus/modus-memory`
- Existing MCP integrations continue to target `modus-memory --vault ...`

### Verification

```bash
GOCACHE=/tmp/modus-memory-gocache go test ./...
GOCACHE=/tmp/modus-memory-gocache go build -ldflags="-s -w" -o /tmp/modus-memory-stripped.bin ./cmd/modus-memory
```

A stripped Apple Silicon build of this release line is approximately 7.7 MB. Exact size varies by target platform and build settings.
