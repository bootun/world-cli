---
name: world-machine
description: Use the `world` CLI to persist and recall long-term bitemporal memories and structured entity state in World Machine. Trigger when the user asks to remember a fact, preference, decision, or event, or asks to recall past context, entity details, or what was true at an earlier time (`--as-of`).
---

# World Machine CLI (`world`)

Use `world` as your external bitemporal memory store instead of inventing ad-hoc curl requests or scattering notes in local files.

## Pre-flight check

Before writing or recalling, verify configuration and credentials if you are unsure whether `world` is logged in:

```sh
world auth status --json
```

If `configured` is `false` or `authenticated` is `false`:
- Ask the user to sign in by running `world auth login` (browser authorization) or `world auth login --api-key "<key>"`.
- Never ask the user to paste secrets directly into tracked repository files.

## Core workflows

### 1. Record a memory (`world remember`)

Use when the user asks you to remember a fact, preference, decision, or event:

```sh
world remember "<verbatim statement>" --source agent --json
```

Rules:
- **Keep original phrasing, especially relative time** (e.g., "yesterday afternoon", "昨天下午", "last Friday"). Do not rewrite relative expressions into approximate timestamps; `world` automatically attaches the local IANA `--timezone` so the server resolves them against the speaker's day.
- Use `--occurred-at` (RFC3339 or `YYYY-MM-DD`) only when the user specifies an explicit historical event date that is not already clear from the sentence.
- Use `--meta key=value` for lightweight structured context (e.g., `--meta project=atlas`).
- `world` automatically generates an `Idempotency-Key` per invocation and safely retries transient errors. Pass `--idempotency-key <key>` if you are replaying a known event.

### 2. Recall memories and entity state (`world recall`)

Use before answering questions that depend on prior user/project context or historical state:

```sh
world recall "<natural-language query>" --json
```

Rules:
- Always pass `--json` so you inspect structured `entities` (`canonicalName`, `entityType`, `components` with `payload`, `confidence`, `epistemicStatus`, `validFromMs`, `validToMs`) and raw `memories`.
- Pay attention to `epistemicStatus` (e.g., observed fact vs stated claim vs inferred belief) and `confidence` when presenting findings.
- For **historical / point-in-time questions** ("what was X's role in Jan 2026?"), pass `--as-of`:

```sh
world recall "<query>" --as-of "2026-01-15" --json
```

### 3. Inspect a specific entity (`world entity get`)

When a recall result returns an `entityId` and you need its full component state (or want to inspect it at a specific instant):

```sh
world entity get <entity_id> [--as-of "2026-01-15"] --json
```

## Security & privacy guardrails

- **Never** read, print, or copy `~/.config/world/config.json` or `WORLD_API_KEY` into chat output, logs, or source files.
- Always use the `world` CLI commands rather than crafting ad-hoc HTTP requests.
- Full flag details and exit codes are in [references/cli-reference.md](references/cli-reference.md).
