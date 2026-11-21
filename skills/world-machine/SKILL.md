---
name: world-machine
description: Persistent cross-session memory via the `world` CLI across all domains and conversations. Load and use proactively without waiting to be asked—(1) recall relevant context, preferences, past choices, or point-in-time history (`--as-of`) BEFORE starting substantive work, and (2) remember durable facts, preferences, constraints, settled decisions, completed outcomes, status changes, relationships, or corrections before finishing, even if also written to local files.
---

# World Machine CLI (`world`)

Use `world` as your external bitemporal memory store across all sessions and domains instead of scattering notes in local files or waiting for explicit "remember" / "recall" commands.

Default lifecycle on every substantive task: **Recall first → Do the work → Remember before finishing**.

## Core operating principles

1. **Recall before starting concrete work**: Before beginning any substantive task (planning, recommending, researching, troubleshooting, drafting, building, or continuing ongoing work), run a quick `world recall` first so existing preferences, constraints, prior decisions, people, and history inform your work from step one. Skip recall only for greetings or self-contained trivia.
2. **Remember before finishing (quietly, without asking permission)**: Before sending your final answer after any non-trivial turn, pause and ask: *"If a fresh session starts tomorrow, what durable fact, preference, constraint, settled decision, completed outcome, status change, relationship, or correction here would save re-investigating or re-asking?"* If anything qualifies under the 5 high-value signals below, run `world remember` quietly before replying—even if the result was also saved to local files. Do **not** interrupt the workflow to ask *"Should I save this to World Machine?"*. Split independent conclusions into separate calls instead of collapsing everything into one vague sentence.
3. **Call directly, handle auth on exit code `3`**: Do **not** run `world auth status` before every read or write. Execute `world recall` or `world remember` directly; only if a command fails with exit code `3` (unauthenticated), run `world auth status --json` and ask the user to sign in via `world auth login` (or `world auth login --no-browser` in SSH/headless sessions).

## Core workflow

### Step 1: Recall context before starting work (`world recall`)

Run `world recall` **before** planning, recommending, executing substantive work, or asking the user to repeat background context whenever:
- You are starting a concrete task where existing user preferences, constraints, prior choices, ongoing plans, or known entities could improve the result.
- The user references a person, project, item, `"之前"`, `"上次"`, or prior decision not fully defined in the current session.
- The user asks about current or historical status.

```sh
world recall "<natural-language query>" --max-entities 8 --max-events 10 --json
```

Rules:
- Always pass `--json` (and bound `--max-entities` / `--max-events` to keep context concise) so you inspect structured `entities` (`canonicalName`, `entityType`, `components` with `payload`, `confidence`, `epistemicStatus`, `validFromMs`, `validToMs`) alongside raw `memories`.
- Pay attention to `epistemicStatus` (e.g., `observed_fact`, `stated_claim`, `inferred_belief`, `disputed_claim`) and `confidence` when presenting findings or taking high-stakes actions.
- For **historical / point-in-time questions** ("what was X's status in Jan 2026?", "上个月的方案是什么"), pass `--as-of`:

```sh
world recall "<query>" --as-of "2026-01-15" --json
```

#### Inspect a specific entity (`world entity get`)

When a recall result returns an `entityId` and you need its full component state (or want to inspect its snapshot at a specific instant):

```sh
world entity get <entity_id> [--as-of "2026-01-15"] --json
```

---

### Step 2: Record durable memories before finishing (`world remember`)

After completing the work—and **before** sending your final reply—persist anything worth carrying into future sessions:

```sh
world remember "<self-contained statement>" --source agent --json
```

#### When to proactively remember (even if the user didn't say "remember")

Record whenever any of these **5 high-value signals** appear in conversation—even if the same details were also written to local files:
1. **User profile, preferences & constraints**: Role, habits, communication or workflow preferences, budgets, hard boundaries, or standing rules.
2. **Settled decisions & completed outcomes**: Finalized choices, committed plans, finished milestones, or concrete actions already completed (not tentative ideas).
3. **State changes & factual corrections**: Updates to status, schedules, ownership, availability, or corrections to previous assumptions. Write an explicit correction statement (e.g., `"Correction: Project Atlas delivery moved to Dec 20, no longer Dec 15."`) so World Machine advances the entity's valid time window.
4. **People, relationships & ongoing entities**: People, teams, organizations, ongoing projects, systems, or assets the user deals with and how they relate (excluding secrets).
5. **Resolved conclusions & root causes**: The settled conclusion, root cause, or takeaway after a non-trivial investigation (record the outcome, not the intermediate trial-and-error trail).

#### When NOT to record (anti-noise filter)

Do **not** call `world remember` for:
1. **Ephemeral instructions & chitchat**: One-off prompts ("translate this paragraph", "rename this variable", "run the tests"), greetings, or general reference knowledge.
2. **Unsettled brainstorming & raw noise**: Tentative ideas ("maybe we could try X?"), intermediate search outputs, stack traces, or raw logs before a conclusion is reached.
3. **Transient local trivia**: Temporary scratch details or routine info that has no value in future sessions.
4. **Same-session duplicates**: Facts you have already recorded earlier in the current conversation.
5. **Secrets & sensitive credentials**: API keys, tokens, passwords, private keys, or connection strings containing credentials.

#### Phrasing rules for accurate bitemporal extraction

- **Self-contained subjects + original relative time**: Resolve pronouns so the sentence stands alone outside the chat, while **preserving original phrasing and relative time expressions** (`"yesterday afternoon"`, `"昨天下午"`, `"last Friday"`). For example, if the user replies *"昨天下午开会决定把截止时间改到周五"*, record:
  `"昨天下午开会决定，将截止时间改为周五。"`
  Never manually convert relative expressions into estimated dates; `world` automatically sends the local IANA `--timezone` so the server resolves them against the speaker's calendar.
- **One distinct event per invocation**: If multiple independent facts or changes with different timelines emerge, split them into separate `world remember` calls.
- **Historical backfill (`--occurred-at`)**: Pass `--occurred-at` (RFC3339 or `YYYY-MM-DD`) only when backfilling a past event with an explicit date not already clear from the statement text. Never pass a future `--occurred-at` (>24h ahead); express future plans in the statement text itself.
- **Keep searchable content in the text**: `--meta key=value` is archived on the raw memory only and is **not** indexed or returned by `world recall`. Put all important entity names and context directly in the statement text.

## Security & privacy guardrails

- **Never** read, print, or copy `~/.config/world/config.json` or `WORLD_API_KEY` into chat output, logs, or source files.
- Never ask the user to paste API keys into tracked repository files.
- Always use the `world` CLI commands rather than crafting ad-hoc HTTP requests.
- Full flag details and exit codes are in [references/cli-reference.md](references/cli-reference.md).
