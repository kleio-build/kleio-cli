#!/usr/bin/env bash
# Kleio: detect MCP/auth failures after MCP tool use (OpenCode hook).
# OpenCode does not yet ship native hook hooks like Claude/Cursor; this script
# is provided so users can wire it into their shell pipeline (e.g. as a
# post-tool wrapper) and so kleio-eval can run it as a watchdog. Safe no-op
# when stdin contains nothing Kleio-related.
set -euo pipefail
INPUT="$(cat || true)"
if ! printf '%s' "$INPUT" | grep -qi 'kleio'; then
  exit 0
fi
if printf '%s' "$INPUT" | grep -qiE '401|403|authentication|auth required|invalid or expired|kleio login|unauthorized'; then
  printf '%s\n' "Kleio auth expired — run \`kleio login\` in a terminal. Restart OpenCode or reload MCP if tokens were just refreshed." >&2
fi
exit 0
