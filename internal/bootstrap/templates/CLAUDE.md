# Kleio (Claude Code)

This project uses [Kleio](https://kleio.build) to capture work items, decisions, and relational checkpoints.

- Prefer the Kleio CLI for relational checkpoints (`kleio checkpoint`) and decisions (`kleio decide`).
- See `AGENTS.md` for shared agent guidance. If this file conflicts with your setup, merge `CLAUDE.kleio.yaml` from `kleio init` instead.

## Claude Code hooks (Kleio)

After `kleio init --tool=claude`, merge **`.claude/settings.json`** (or the `settings.kleio.json` sidecar) into your Claude Code settings. It adds **SessionStart** / **Stop** reminders and **PostToolUseFailure** auth detection via `.claude/hooks/kleio-auth-check.sh` (run `chmod +x` on that script).

## Kleio MCP over HTTP

For cloud or headless agents, configure MCP to **`POST https://<your-api-host>/api/mcp`** with `Authorization: Bearer <access_token>` and header **`X-Workspace-ID: <workspace_id>`** (same workspace as the REST API).

## Kleio MCP auth after `kleio login`

The `kleio mcp` process polls `~/.kleio/config.yaml` about every 30 seconds and reapplies tokens and workspace from disk, so you usually do not need to restart Claude Code after logging in. If something still fails, restart the MCP server.
