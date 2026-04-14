# Kleio (GitHub Copilot)

This project uses [Kleio](https://kleio.build) for durable engineering signals (decisions, checkpoints, backlog).

- Follow **`AGENTS.md`** for when to call Kleio tools (`kleio_capture`, `kleio_checkpoint`, `kleio_decide`, `kleio_session_summary`, etc.).
- Repo hooks under **`.github/hooks/kleio-hooks.json`** remind the agent after MCP failures and at session start.
- For **cloud / remote** setups, prefer **HTTP MCP** to `https://<your-kleio-api-host>/api/mcp` with `Authorization: Bearer <access_token>` and header **`X-Workspace-ID: <workspace_id>`** (see Kleio docs and `kleio init` Cursor MCP example).
