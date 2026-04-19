# Kleio guidance for OpenCode

OpenCode reads `AGENTS.md` for project-level instructions. This sidecar file is
installed by `kleio init --surface opencode` so you can opt-in to Kleio's
agent guidance without overwriting your existing `AGENTS.md`.

## What Kleio expects of agents

1. **Log decisions before implementing.** When an agent picks a non-trivial
   direction, call `kleio_decide` (MCP) or `kleio decide` (CLI) with
   `content`, `rationale`, `confidence`, and any `alternatives`.

2. **Capture follow-up work.** When an agent discovers actionable work it
   won't do this turn, call `kleio_capture` (defaults to `signal_type =
   work_item`). Do NOT use it for checkpoints or decisions — those have
   dedicated tools.

3. **Checkpoint at slice completion.** When a meaningful slice ships, call
   `kleio_checkpoint` with `slice_category`, `slice_status`, and
   `validation_status`. Optionally link a backlog item via
   `backlog_item_id`.

## MCP wiring

This bootstrap installs `opencode.json.example` (local stdio) and
`opencode.http.json.example` (HTTP). Merge the one you want into your
project or `~/.config/opencode/opencode.json`:

```json
{
  "$schema": "https://opencode.ai/config.json",
  "mcp": {
    "kleio": {
      "type": "local",
      "command": ["kleio", "mcp"],
      "enabled": true
    }
  }
}
```

For cloud sessions or shared eval workspaces, use the `remote` form with a
bearer token + `X-Workspace-ID` header.

## Hooks

`opencode/hooks/kleio-auth-check.sh` is a portable bash watchdog you can wire
into a shell pipeline that wraps OpenCode tool output. It surfaces 401/403
auth failures from Kleio MCP responses as actionable error messages.

## Troubleshooting

- `kleio status` — verify auth + workspace + MCP discovery.
- `kleio query captures --query "..."` — sanity-check that captures are
  flowing into the workspace OpenCode is targeting.
