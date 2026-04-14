#!/usr/bin/env bash
# Kleio: SessionStart / Stop nudges for Codex (JSON on stdout).
# Avoids a hard jq dependency: uses jq if present, else python3 (only if it runs), else tiny grep heuristics + fixed JSON.
set -euo pipefail

INPUT="$(cat || true)"
[ -z "$INPUT" ] && exit 0

python3_ok() {
  command -v python3 >/dev/null 2>&1 && python3 -c 'import json' >/dev/null 2>&1
}

detect_event() {
  local ev=""
  if command -v jq >/dev/null 2>&1; then
    ev="$(printf '%s' "$INPUT" | jq -r '.hook_event_name // empty' 2>/dev/null || true)"
    printf '%s' "$ev"
    return 0
  fi
  if python3_ok; then
    ev="$(printf '%s' "$INPUT" | python3 -c '
import json, sys
try:
    d = json.load(sys.stdin)
    print(d.get("hook_event_name") or "")
except Exception:
    print("")
' 2>/dev/null || true)"
    printf '%s' "$ev"
    return 0
  fi
  if printf '%s' "$INPUT" | grep -qE '"hook_event_name"[[:space:]]*:[[:space:]]*"Stop"'; then
    printf '%s' Stop
    return 0
  fi
  if printf '%s' "$INPUT" | grep -qE '"hook_event_name"[[:space:]]*:[[:space:]]*"SessionStart"'; then
    printf '%s' SessionStart
    return 0
  fi
  printf ''
}

emit_stop() {
  if command -v jq >/dev/null 2>&1; then
    jq -n --arg msg 'Kleio: If Kleio MCP is enabled, call kleio_session_summary before ending the turn and log gaps with kleio_capture / kleio_checkpoint / kleio_decide.' \
      '{continue:true, hookSpecificOutput:{hookEventName:"Stop", additionalContext:$msg}}'
    return
  fi
  if python3_ok; then
    python3 -c 'import json; print(json.dumps({"continue": True, "hookSpecificOutput": {"hookEventName": "Stop", "additionalContext": "Kleio: If Kleio MCP is enabled, call kleio_session_summary before ending the turn and log gaps with kleio_capture / kleio_checkpoint / kleio_decide."}}))'
    return
  fi
  printf '%s\n' '{"continue":true,"hookSpecificOutput":{"hookEventName":"Stop","additionalContext":"Kleio: If Kleio MCP is enabled, call kleio_session_summary before ending the turn and log gaps with kleio_capture / kleio_checkpoint / kleio_decide."}}'
}

emit_session_start() {
  if command -v jq >/dev/null 2>&1; then
    jq -n --arg msg 'Kleio: If Kleio MCP is enabled, call kleio_session_summary once early in this session.' \
      '{hookSpecificOutput:{hookEventName:"SessionStart", additionalContext:$msg}}'
    return
  fi
  if python3_ok; then
    python3 -c 'import json; print(json.dumps({"hookSpecificOutput": {"hookEventName": "SessionStart", "additionalContext": "Kleio: If Kleio MCP is enabled, call kleio_session_summary once early in this session."}}))'
    return
  fi
  printf '%s\n' '{"hookSpecificOutput":{"hookEventName":"SessionStart","additionalContext":"Kleio: If Kleio MCP is enabled, call kleio_session_summary once early in this session."}}'
}

ev="$(detect_event | tail -n1)"
case "$(printf '%s' "$ev" | tr -d '\r\n')" in
  Stop) emit_stop ;;
  SessionStart) emit_session_start ;;
esac
exit 0
