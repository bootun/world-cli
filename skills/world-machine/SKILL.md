---
name: world-machine
description: Proactively persist and recall long-term bitemporal memories and structured entity state using the `world` CLI. Trigger not only when the user explicitly asks to remember or recall, but also whenever durable facts, user preferences, architectural/project decisions, ownership changes, or factual corrections surface in conversation, or when prior context/historical state (`--as-of`) is needed.
---

# World Machine CLI (`world`)

Use `world` as your external bitemporal memory store across sessions instead of scattering notes in local files or waiting for explicit "remember this" commands.

## Core operating principles

1. **Act without asking permission**: When a durable fact or decision surfaces during conversation, run `world remember` quietly alongside your main task. Do **not** interrupt the workflow to ask "Should I save this to World Machine?".
2. **Call directly, handle auth on exit code `3`**: Do **not** run `world auth status` before every read or write. Execute `world remember` or `world recall` directly; only if a command fails with exit code `3` (unauthenticated), run `world auth status --json` and ask the user to sign in via `world auth login` (or `world auth login --no-browser` in SSH/headless sessions).

## Core workflows

### 1. Proactively record memories (`world remember`)

```sh
world remember "<self-contained statement>" --source agent --json
```

#### When to proactively remember (even if the user didn't say "remember")

Record immediately when any of these **5 high-value signals** appear in conversation:
1. **User profile & durable preferences**: Role, team, location, tech-stack preferences, coding standards, or workflow rules (e.g., "always run `make verify` before committing", "never generate mock tests for DB adapters").
2. **Architectural, technical & project decisions**: Settled choices on frameworks, module boundaries, API contracts, business rules, or release milestones/deadlines.
3. **State transitions & factual corrections**: Status changes (service deprecated, deadline moved, role handed over) or user corrections to previous assumptions. Write a clear correction statement (e.g., `"Correction: Project Atlas delivery moved to Dec 20, no longer Dec 15."`) so World Machine advances the entity's valid time window.
4. **People, ownership & system topology**: Who owns which service/domain, team relationships, and non-secret environment/service mappings.
5. **Settled root-cause conclusions**: Final root cause and fix of a non-trivial bug or production incident after investigation finishes (record the conclusion, not the debugging trail).

#### When NOT to record (anti-noise filter)

Do **not** call `world remember` for:
1. **Ephemeral task instructions & chitchat**: One-off prompts ("rename this variable", "fix line 42", "run the tests"), greetings, or general programming knowledge.
2. **Unsettled brainstorming & debugging noise**: Tentative ideas ("maybe we could try X?"), intermediate stack traces, compiler errors, or raw log dumps before a conclusion is reached.
3. **Static repo trivia**: Line numbers, local function signatures, or routine details already obvious from reading the current repository's files.
4. **Same-session duplicates**: Facts you have already recorded earlier in the current conversation.
5. **Secrets & sensitive credentials**: API keys, tokens, passwords, private keys, or connection strings containing credentials.

#### Phrasing rules for accurate bitemporal extraction

- **Self-contained subjects + original relative time**: Resolve pronouns so the sentence stands alone outside the chat, while **preserving original phrasing and relative time expressions** (`"yesterday afternoon"`, `"昨天下午"`, `"last Friday"`). For example, if the user replies *"昨天下午架构会决定改成 30 秒"*, record:
  `"昨天下午架构会决定，将订单服务的缓存 TTL 改为 30 秒。"`
  Never manually convert relative expressions into estimated dates; `world` automatically sends the local IANA `--timezone` so the server resolves them against the speaker's calendar.
- **One distinct event per invocation**: If the user mentions multiple independent changes with different timelines, split them into separate `world remember` calls.
- **Historical backfill (`--occurred-at`)**: Pass `--occurred-at` (RFC3339 or `YYYY-MM-DD`) only when backfilling a past event with an explicit date not already clear from the statement text. Never pass a future `--occurred-at` (>24h ahead); express future plans in the statement text itself.
- **Keep searchable content in the text**: `--meta key=value` is archived on the raw memory only and is **not** indexed or returned by `world recall`. Put all important entity names and context directly in the statement text.

### 2. Proactively recall context and entity state (`world recall`)

Run `world recall` **before** guessing or asking the user to repeat background context whenever:
- The user mentions a project codename, person, system, `"按之前的约定"`, or `"上次说的方案"` not defined in the current session.
- You are about to make a design/tooling recommendation where prior user preferences or project decisions might apply.
- The user asks about current or historical status.

```sh
world recall "<natural-language query>" --max-entities 8 --max-events 10 --json
```

Rules:
- Always pass `--json` (and bound `--max-entities` / `--max-events` when appropriate to keep context concise) so you inspect structured `entities` (`canonicalName`, `entityType`, `components` with `payload`, `confidence`, `epistemicStatus`, `validFromMs`, `validToMs`) alongside raw `memories`.
- Pay attention to `epistemicStatus` (e.g., `observed_fact`, `stated_claim`, `inferred_belief`, `disputed_claim`) and `confidence` when presenting findings or taking high-stakes actions.
- For **historical / point-in-time questions** ("what was X's role in Jan 2026?", "上个月的配置是什么"), pass `--as-of`:

```sh
world recall "<query>" --as-of "2026-01-15" --json
```

### 3. Inspect a specific entity (`world entity get`)

When a recall result returns an `entityId` and you need its full component state (or want to inspect its snapshot at a specific instant):

```sh
world entity get <entity_id> [--as-of "2026-01-15"] --json
```

## Security & privacy guardrails

- **Never** read, print, or copy `~/.config/world/config.json` or `WORLD_API_KEY` into chat output, logs, or source files.
- Never ask the user to paste API keys into tracked repository files.
- Always use the `world` CLI commands rather than crafting ad-hoc HTTP requests.
- Full flag details and exit codes are in [references/cli-reference.md](references/cli-reference.md).
