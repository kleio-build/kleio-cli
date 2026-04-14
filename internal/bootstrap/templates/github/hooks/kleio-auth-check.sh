#!/usr/bin/env bash
# Kleio: detect MCP/auth failures (GitHub Copilot agent hooks).
set -euo pipefail
INPUT="$(cat || true)"
if ! printf '%s' "$INPUT" | grep -qi 'kleio'; then
  exit 0
fi
if printf '%s' "$INPUT" | grep -qiE '401|403|authentication|auth required|invalid or expired|kleio login|unauthorized'; then
  printf '%s\n' "Kleio auth expired — run \`kleio login\` locally and refresh tokens used by the Copilot agent / MCP config." >&2
fi
exit 0
