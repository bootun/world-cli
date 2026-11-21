# `world` CLI Reference

## Exit codes

- `0`: Success
- `1`: General, network, or server error
- `2`: Usage or invalid argument error
- `3`: Unauthenticated (missing or invalid API key; prompt user to run `world auth login`)
- `4`: Resource not found

## Commands

### Authentication & configuration

- `world auth login` — Open browser to `console.worldmachine.dev/cli-auth` and receive a dedicated API key over a localhost callback.
- `world auth login --no-browser` — Print the authorization URL instead of launching a browser.
- `world auth login --api-key <key> [--endpoint <url>]` — Save an existing API key directly.
- `world auth status [--offline] [--json]` — Inspect config, check `/healthz`, and probe key validity.
- `world auth logout` — Clear stored credentials from the active profile.
- `world config list [--json]` — Print resolved configuration.
- `world config get <key>` — Read `profile`, `endpoint`, `console-url`, `api-key` (masked), `timezone`, `source`, or `output`.
- `world config set <key> <value>` — Update a profile setting or switch `profile`.

### Memories & entities

- `world remember "<text>" [--source <label>] [--timezone <iana>] [--occurred-at <time>] [--meta k=v] [--idempotency-key <key>] [--json]`
  - Pass `-` or pipe stdin to write multi-line text.
  - `--meta k=v` is stored on the raw memory archive only and is not indexed or returned by `recall`.
  - Response JSON: `{"deduplicated":false}`.
- `world recall "<query>" [--as-of <time>] [--max-entities <n>] [--max-events <n>] [--timezone <iana>] [--json]`
  - `--as-of` accepts RFC3339, `YYYY-MM-DD`, `YYYY-MM-DD HH:MM:SS`, or Unix ms/s.
  - Response JSON: `{"entities":[...],"memories":[...]}`.
- `world entity get <entity_id> [--as-of <time>] [--timezone <iana>] [--json]`
  - Response JSON: `{"entity":{...}}`.

### AI Harness lifecycle hooks

- `world hook install [--target auto|all|antigravity|claude|codex] [--scope user|project] [--command <bin>] [--json]` — Register `SessionStart` (recall-first reminder) and `Stop` (end-of-turn memory review) hooks into local AI harness configs without overwriting unrelated settings.
- `world hook status [--target ...] [--scope user|project] [--json]` — Show hook installation status across supported harnesses.
- `world hook uninstall [--target ...] [--scope user|project] [--json]` — Remove World Machine hooks cleanly.
- `world hook session-start` / `world hook stop` — Runtime callback subcommands invoked automatically by the AI harness over stdin/stdout (auto-detects Antigravity vs. Claude Code/Codex JSON dialects).

## Environment variables

- `WORLD_API_KEY` (or `WM_API_KEY`) — API key override
- `WORLD_ENDPOINT` — API endpoint override (default `https://api.worldmachine.dev`)
- `WORLD_CONSOLE_URL` — Console URL override (default `https://console.worldmachine.dev`)
- `WORLD_PROFILE` — Active profile override
- `WORLD_TIMEZONE` — Default IANA timezone override
- `WORLD_SOURCE` — Default source label override
