#!/usr/bin/env bash
# Kleio: detect MCP/auth failures (Claude Code PostToolUseFailure).
set -euo pipefail
INPUT="$(cat || true)"
if ! printf '%s' "$INPUT" | grep -qiE 'mcp__.*kleio|user-kleio|kleio'; then
  exit 0
fi
if printf '%s' "$INPUT" | grep -qiE '401|403|authentication|auth required|invalid or expired|kleio login|unauthorized'; then
  printf '%s\n' "Kleio auth expired — run \`kleio login\` in a terminal. The \`kleio mcp\` process reloads ~/.kleio/config.yaml about every 30 seconds; restart MCP if it still fails." >&2
fi
exit 0
