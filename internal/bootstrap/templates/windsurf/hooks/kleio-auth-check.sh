#!/usr/bin/env bash
# Kleio: detect MCP/auth failures after MCP tool use (Windsurf post_mcp_tool_use).
set -euo pipefail
INPUT="$(cat || true)"
if ! printf '%s' "$INPUT" | grep -qi 'kleio'; then
  exit 0
fi
# tool_info.mcp_result may be a string or JSON blob
if printf '%s' "$INPUT" | grep -qiE '401|403|authentication|auth required|invalid or expired|kleio login|unauthorized'; then
  printf '%s\n' "Kleio auth expired — run \`kleio login\` in a terminal. Restart Windsurf or reload MCP if tokens were just refreshed." >&2
fi
exit 0
