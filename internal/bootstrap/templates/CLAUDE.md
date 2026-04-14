# Kleio (Claude Code)

This project uses [Kleio](https://kleio.build) to capture work items, decisions, and relational checkpoints.

- Prefer the Kleio CLI for relational checkpoints (`kleio checkpoint`) and decisions (`kleio decide`).
- See `AGENTS.md` for shared agent guidance. If this file conflicts with your setup, merge `CLAUDE.kleio.yaml` from `kleio init` instead.

## Kleio MCP auth after `kleio login`

The `kleio mcp` process polls `~/.kleio/config.yaml` about every 30 seconds and reapplies tokens and workspace from disk, so you usually do not need to restart Claude Code after logging in. If something still fails, restart the MCP server.
