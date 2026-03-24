# Kleio Cursor / agent prepackage

Template files for teams using Kleio with Cursor and the Kleio MCP server. This folder is the **distributable bundle** described in the Kleio consumer-packaging decision (canonical rule + `AGENTS.md` + optional Cursor skill + config examples).

**Note:** Cursor-specific files live under **`cursor/`** in this tree (not `.cursor/`) because Go `embed` skips hidden directories; `kleio init` copies them into **`.cursor/`** in your repo.

## Install in your repository

1. Copy **`AGENTS.md`** to your project root.
2. Copy **`.cursor/rules/kleio-mcp.mdc`** into your project (create `.cursor/rules/` if needed).
3. Optionally copy Cursor skills from **`.cursor/skills/`**: **`kleio-decision-logging/`** (when to log `kleio_decide`) and **`kleio-checkpoint-logging/`** (when to use `kleio_checkpoint` vs smart capture).
4. Copy **`kleio.config.example.yaml`** to **`~/.kleio/config.yaml`** (Windows: **`%USERPROFILE%\.kleio\config.yaml`**) and adjust `api_url`, `api_key`, and `workspace_id` for your environment.
5. Merge **`mcp.json.example`** into your Cursor MCP config (often `~/.cursor/mcp.json`), pointing `command` at your `kleio` binary (`go install` path or full path on Windows).
6. Restart Cursor.

## Compatibility

Checkpoint support requires a **`kleio` binary** that includes `kleio checkpoint` and MCP **`kleio_checkpoint`** (MCP server reports version **0.3.0** or newer). Upgrade with `go install github.com/kleio-build/kleio-cli/cmd/kleio@<version>` (or your tagged release), then restart Cursor so the MCP server picks up the new binary.

## kleio-build note

In this monorepo, the root **`AGENTS.md`**, **`.cursor/rules/kleio-mcp.mdc`**, and **`.cursor/skills/`** (decision + checkpoint logging) should stay in sync with the copies in **`kleio-config/prepackage/`** when you change the template.
