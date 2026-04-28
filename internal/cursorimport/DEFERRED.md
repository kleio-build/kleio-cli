# Deferred Tool Parsers

These parsers were deferred from Phase 2 of the local signal mining plan.
Cursor is the happy-path implementation; these follow the same pattern.

## Claude Code — `kleio import claude`

- **Location**: `~/.claude/projects/<hash>/<session-id>.jsonl`
- **Format**: JSONL, similar structure to Cursor but different schema (`type`/`role`/`content` fields instead of `message.content` array)
- **Additional artifacts**: `memory/*.md` auto-memory files
- **Parser approach**: Follow the `cursorimport` pattern (discover → parse → dedup → import). Reuse `internal/privacy` filter.
- **Priority**: Medium — JSONL is trivially parseable; main work is schema mapping.

## Codex CLI — `kleio import codex`

- **Location**: `~/.codex/sessions/YYYY/MM/DD/rollout-*.jsonl`
- **Format**: JSONL with `type`/`item`/`seq` structure, organized by date
- **Parser approach**: Follow the `cursorimport` pattern. New file per session (not append-only like Cursor).
- **Priority**: Medium — different schema but same fundamental approach.

## Aider — `kleio import aider`

- **Location**: `.aider.chat.history.md` in project root
- **Format**: Markdown dialog (NOT JSONL)
- **Parser approach**: Requires fundamentally different parser (markdown block extraction).
- **Priority**: Low — Markdown parsing is more complex and error-prone.

## Copilot — Skipped

- **Location**: `state.vscdb` (SQLite)
- **Reason**: Undocumented schema, WAL mode, file lock issues. No standardized local session store.
- **Priority**: Not planned.
