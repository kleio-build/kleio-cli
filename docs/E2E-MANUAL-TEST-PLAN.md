# E2E Manual Testing Plan — CLI Local-First Pivot

> **Target:** fresh repo, fresh install, clean `~/.kleio/` state
> **Environment:** staging (`api.dev.kleio.build`) or local dev (`localhost:8080`)
> **Branch:** `feature/sync` (contains all phases stacked)
> **Time estimate:** ~45 minutes for full run

---

## Prerequisites

```bash
# 1. Build from source (feature/sync has all changes)
cd kleio-cli
git checkout feature/sync
go build -o kleio ./cmd/kleio

# 2. Put the binary on PATH (or use ./kleio everywhere)
export PATH="$PWD:$PATH"

# 3. Create a throwaway test repo
cd /tmp
mkdir kleio-e2e-test && cd kleio-e2e-test
git init -b main
git config user.email "tester@example.com"
git config user.name "Test User"

# 4. Seed the repo with realistic commits (copy-paste block)
echo "# E2E Test Repo" > README.md && git add . && git commit -m "initial commit"
echo "package auth" > auth.go && git add . && git commit -m "feat: add auth module PROJ-101"
echo "package auth_test" > auth_test.go && git add . && git commit -m "test: add auth tests (#3)"
mkdir -p internal/payments
echo "package payments" > internal/payments/checkout.go && git add . && git commit -m "feat: checkout flow for ENG-42"
echo "// fix" >> internal/payments/checkout.go && git add . && git commit -m "fix: checkout returns 500 on empty cart"
echo "package db" > db.go && git add . && git commit -m "refactor: extract database layer"
echo "// v1" >> README.md && git add . && git commit -m "chore: prep release v1.0.0"
echo "package search" > search.go && git add . && git commit -m "feat: add search endpoint KL-7"

# 5. Clear any existing kleio state
rm -rf .kleio
rm -rf ~/.kleio  # WARNING: removes all kleio config — skip if you want to preserve auth
```

---

## Section 1: Local Mode (zero auth, zero config)

Everything in this section must work **without** `kleio login`, `~/.kleio/config.yaml`, or any environment variables.

### 1.1 First boot — verify no crash without config

| # | Step | Expected |
|---|------|----------|
| 1 | `kleio --version` | Prints version string, exit 0 |
| 2 | `kleio --help` | Shows all commands including `trace`, `explain`, `incident`, `sync` |
| 3 | `ls .kleio/` | Should not exist yet |

### 1.2 Local capture — write and read without auth

| # | Step | Expected |
|---|------|----------|
| 4 | `kleio capture "fix: auth token expiry needs investigation"` | Succeeds, prints event JSON. `.kleio/kleio.db` is auto-created |
| 5 | `ls .kleio/kleio.db` | File exists |
| 6 | `kleio capture "debt: checkout error handling is brittle" --repo test-repo --branch main` | Succeeds with repo/branch metadata |
| 7 | `kleio query captures` | Lists both captured events (most recent first) |
| 8 | Copy the ID from event #1, run: `kleio query capture <ID>` | Shows full event detail |

### 1.3 Local checkpoint — structured metadata stored locally

| # | Step | Expected |
|---|------|----------|
| 9 | `kleio checkpoint "auth module completed" --slice-category implementation --slice-status completed --validation-status passed --checkpoint-file auth.go --checkpoint-file auth_test.go` | Succeeds, prints event with `signal_type: checkpoint` |
| 10 | `kleio query captures` | Now shows 3 events: the checkpoint + 2 captures |

### 1.4 Local decision — decision with alternatives

| # | Step | Expected |
|---|------|----------|
| 11 | `kleio decide "Use JWT for session tokens" --rationale "Stateless, no session store needed" --confidence high --alternative "Session cookies" --alternative "OAuth opaque tokens"` | Succeeds, prints event with `signal_type: decision` |
| 12 | `kleio query captures` | Shows 4 events total |

### 1.5 Local backlog

| # | Step | Expected |
|---|------|----------|
| 13 | `kleio backlog list` | Returns empty list or any items from local store |

### 1.6 Local search

| # | Step | Expected |
|---|------|----------|
| 14 | `kleio query semantic "auth"` | Returns matching events containing "auth" |
| 15 | `kleio query semantic "nonexistent-term-xyz"` | Returns empty results, no error |

---

## Section 2: Git Indexer

### 2.1 Full index of test repo

| # | Step | Expected |
|---|------|----------|
| 16 | Verify the test repo has commits: `git log --oneline` | Shows ~8 commits |
| 17 | The DB should already exist from captures above. Verify commits table is empty: `sqlite3 .kleio/kleio.db "SELECT COUNT(*) FROM commits;"` | `0` (not yet indexed) |
| 18 | (Note: `kleio init` in current form installs templates; the indexer runs via internal code. For testing, we'll use `kleio scan` which triggers indexing, or we verify via the trace/explain commands that auto-index on first use.) | |

### 2.2 Identifier extraction

| # | Step | Expected |
|---|------|----------|
| 19 | After indexing, check identifiers: `sqlite3 .kleio/kleio.db "SELECT kind, value, provider FROM identifiers;"` | Should show: `PROJ-101` (jira), `#3` (github PR), `ENG-42` (jira), `KL-7` (kleio), `v1.0.0` (git_tag) |
| 20 | Check links exist: `sqlite3 .kleio/kleio.db "SELECT COUNT(*) FROM links;"` | Greater than 0 |

### 2.3 Incremental indexing

| # | Step | Expected |
|---|------|----------|
| 21 | Add a new commit: `echo "// new" >> auth.go && git add . && git commit -m "fix: auth token refresh bug PROJ-102"` | |
| 22 | Trigger re-index (via trace or other command that touches the indexer) | |
| 23 | `sqlite3 .kleio/kleio.db "SELECT COUNT(*) FROM commits;"` | Count increased by 1 |
| 24 | Check for `PROJ-102`: `sqlite3 .kleio/kleio.db "SELECT value FROM identifiers WHERE value='PROJ-102';"` | Returns `PROJ-102` |

---

## Section 3: Trigger Commands (the activation commands)

### 3.1 `kleio trace`

| # | Step | Expected |
|---|------|----------|
| 25 | `kleio trace "auth"` | Prints trace report with timeline of commits mentioning "auth", segmented by date. Shows commits like "feat: add auth module" and "fix: auth token refresh bug" |
| 26 | `kleio trace auth.go` | Shows timeline specific to that file path |
| 27 | `kleio trace --since 7d "checkout"` | Shows only recent results (our test commits are recent, so should return results) |
| 28 | `kleio trace "auth" --json` | Valid JSON output, parseable by `jq` if available |
| 29 | `kleio trace "totally-nonexistent-thing-12345"` | Exit code 1 (no results), stderr message "No results found" |
| 30 | `echo | kleio trace "auth" --no-interactive` | No prompts, outputs results directly |

### 3.2 `kleio explain`

| # | Step | Expected |
|---|------|----------|
| 31 | `kleio explain HEAD~5 HEAD` | Shows explanation report with commit count, subsystems, and change details |
| 32 | `kleio explain HEAD~5 HEAD --json` | Valid JSON output |
| 33 | `kleio explain HEAD~5 HEAD --no-interactive` | No prompts |

### 3.3 `kleio incident`

| # | Step | Expected |
|---|------|----------|
| 34 | `kleio incident "checkout returns 500"` | Shows incident report with suspicious commits ranked by relevance. The "fix: checkout returns 500 on empty cart" commit should rank highest |
| 35 | `kleio incident --files internal/payments/checkout.go` | Narrows to commits touching that file path |
| 36 | `kleio incident --since 7d "error"` | Shows recent error-related commits |
| 37 | `kleio incident "checkout returns 500" --json` | Valid JSON with `suspects` array |
| 38 | `kleio incident "completely unrelated xyz"` | Exit code 1, "No suspicious changes found" message |

### 3.4 Shared flag consistency

| # | Step | Expected |
|---|------|----------|
| 39 | For each of `trace`, `explain`, `incident`: verify `--help` shows `--json`, `--no-interactive`, and `--since` | All three commands have all three flags |

---

## Section 4: MCP Server (local mode)

### 4.1 MCP starts without auth

| # | Step | Expected |
|---|------|----------|
| 40 | Ensure no auth config exists: `unset KLEIO_TOKEN; unset KLEIO_API_KEY` | |
| 41 | Start MCP: `echo '{"jsonrpc":"2.0","method":"initialize","id":1,"params":{"capabilities":{},"clientInfo":{"name":"test","version":"1.0"},"protocolVersion":"2024-11-05"}}' \| kleio mcp` | Responds with JSON-RPC `initialize` result containing `kleio-mcp` server info. Does NOT crash for missing auth |

### 4.2 MCP tools work locally

Since MCP uses JSON-RPC over stdio, this is best tested via an actual MCP client (e.g. Cursor). For manual verification:

| # | Step | Expected |
|---|------|----------|
| 42 | Configure Cursor/Claude/etc. to use `kleio mcp` as an MCP server | Server starts, tools visible |
| 43 | In the AI editor, ask the agent to call `kleio_capture` with content "test from mcp" | Event created in local `.kleio/kleio.db` |
| 44 | Ask the agent to call `kleio_backlog_list` | Returns list from local store (may be empty) |
| 45 | Ask the agent to call `kleio_trace` with anchor "auth" | Returns timeline results |
| 46 | Ask the agent to call `kleio_ask` with question "what happened with auth?" | Returns local text search results + "connect to Kleio Cloud" upgrade nudge |
| 47 | Ask the agent to call `kleio_checkpoint` with required fields | Checkpoint created locally |
| 48 | Ask the agent to call `kleio_decide` with content + rationale + confidence | Decision created locally |

---

## Section 5: Cloud Mode (requires auth)

### 5.1 Authenticate against staging

| # | Step | Expected |
|---|------|----------|
| 49 | `kleio config use staging` | Switches to staging env |
| 50 | `kleio login` | Opens browser for GitHub OAuth, completes auth flow |
| 51 | `kleio status` | Shows "Authenticated", workspace info, API connectivity "OK" |

### 5.2 Cloud commands still work (regression)

| # | Step | Expected |
|---|------|----------|
| 52 | `kleio capture "cloud test capture"` | Succeeds, creates capture via API (not SQLite) |
| 53 | `kleio query captures` | Returns results from cloud API |
| 54 | `kleio backlog list` | Returns cloud backlog items |
| 55 | `kleio checkpoint "cloud checkpoint" --slice-category implementation --slice-status completed --validation-status passed` | Creates checkpoint via API |
| 56 | `kleio decide "cloud decision" --rationale "testing" --confidence medium` | Creates decision via API |

### 5.3 MCP in cloud mode

| # | Step | Expected |
|---|------|----------|
| 57 | With auth configured, start MCP via editor | Server starts, connects to cloud API |
| 58 | `kleio_ask` in cloud mode | Returns AI-synthesized answer (not text-search fallback) |
| 59 | `kleio_backlog_prioritize` with `assignee_id: self` | Resolves self via `/api/me` and sets assignee |

---

## Section 6: Sync (local to cloud upgrade)

### 6.1 Setup: create local data, then authenticate

| # | Step | Expected |
|---|------|----------|
| 60 | `kleio logout` | Clears auth |
| 61 | `rm -rf .kleio` | Fresh local state |
| 62 | Create local captures: `kleio capture "local-only item 1"` then `kleio capture "local-only item 2"` | Both stored in SQLite with `synced=0` |
| 63 | Verify synced status: `sqlite3 .kleio/kleio.db "SELECT id, content, synced FROM events;"` | Both rows show `synced=0` |

### 6.2 Push local data to cloud

| # | Step | Expected |
|---|------|----------|
| 64 | `kleio config use staging && kleio login` | Authenticate |
| 65 | `kleio sync push` | Output: "Pushed: 2 event(s), 0 backlog item(s)" |
| 66 | `sqlite3 .kleio/kleio.db "SELECT synced FROM events;"` | Both rows now show `synced=1` |
| 67 | `kleio sync push` (run again) | Output: "Pushed: 0 event(s), 0 backlog item(s)" — idempotent, no duplicates |
| 68 | Verify in cloud: `kleio query captures` (cloud mode should show the pushed items) | Items appear in cloud query results |

### 6.3 Pull cloud data locally

| # | Step | Expected |
|---|------|----------|
| 69 | `kleio sync pull` | Output: "Pulled: N event(s), M backlog item(s)" (N > 0 if there's existing cloud data) |
| 70 | `sqlite3 .kleio/kleio.db "SELECT COUNT(*) FROM events WHERE synced=1;"` | Pulled items have `synced=1` |

### 6.4 Push with --json

| # | Step | Expected |
|---|------|----------|
| 71 | Create another local capture, then `kleio sync push --json` | Valid JSON output with `events_pushed`, `backlog_pushed`, `errors` fields |

### 6.5 Auto sync (smoke test)

| # | Step | Expected |
|---|------|----------|
| 72 | `kleio sync auto --interval 5s` | Starts background sync, prints periodic push/pull stats to stderr. Ctrl-C to stop after ~15s |

---

## Section 7: BYOK LLM (optional enhancement)

> Skip this section if no API keys are available. The CLI must work perfectly without it.

### 7.1 Verify heuristic-only mode

| # | Step | Expected |
|---|------|----------|
| 73 | Ensure no `ai:` block in `~/.kleio/config.yaml` | |
| 74 | `kleio trace "auth"` | Returns heuristic timeline (no LLM call). No error about missing API keys |

### 7.2 Configure BYOK (OpenAI example)

| # | Step | Expected |
|---|------|----------|
| 75 | Add to `~/.kleio/config.yaml`: | |

```yaml
ai:
  provider: openai
  model: gpt-4o-mini
  api_key_env: OPENAI_API_KEY
```

| 76 | `export OPENAI_API_KEY=sk-...` | Set the key |
| 77 | `kleio trace "auth"` | Should produce an LLM-enhanced summary (richer than heuristic). If API fails, falls back to heuristic silently |

### 7.3 Ollama (local LLM)

| # | Step | Expected |
|---|------|----------|
| 78 | Install Ollama, pull a model: `ollama pull llama3` | |
| 79 | Configure: `ai: { provider: ollama, model: llama3 }` | |
| 80 | `kleio trace "auth"` | Uses local Ollama for summarization |

---

## Section 8: Edge Cases and Error Handling

### 8.1 Non-git directory

| # | Step | Expected |
|---|------|----------|
| 81 | `cd /tmp && mkdir not-a-repo && cd not-a-repo` | |
| 82 | `kleio capture "test in non-git dir"` | Should still work — captures go to local SQLite. The DB is created even without git |
| 83 | `kleio trace "anything"` | May return empty results (no git history) — should not crash |

### 8.2 Empty repo (no commits)

| # | Step | Expected |
|---|------|----------|
| 84 | `cd /tmp && mkdir empty-repo && cd empty-repo && git init` | |
| 85 | `kleio trace "test"` | Exit 1, "No results found" — no crash |
| 86 | `kleio incident "error"` | Exit 1, "No suspicious changes" — no crash |

### 8.3 Large output

| # | Step | Expected |
|---|------|----------|
| 87 | In a real repo with 1000+ commits, run `kleio trace "fix"` | Returns results in reasonable time (<5s), output is paginated or capped |

### 8.4 Concurrent access

| # | Step | Expected |
|---|------|----------|
| 88 | In two terminals, simultaneously run `kleio capture "concurrent 1"` and `kleio capture "concurrent 2"` | Both succeed (SQLite WAL mode handles concurrent writes) |

### 8.5 Corrupt / missing database

| # | Step | Expected |
|---|------|----------|
| 89 | `echo "garbage" > .kleio/kleio.db` | |
| 90 | `kleio capture "test after corruption"` | Should error gracefully, not panic |
| 91 | `rm .kleio/kleio.db` then `kleio capture "test after delete"` | Auto-creates a fresh database |

---

## Section 9: Scan commands (existing, regression)

| # | Step | Expected |
|---|------|----------|
| 92 | `kleio scan standup` | Shows today's work summary from git |
| 93 | `kleio scan pr` | Shows PR-style change summary for current branch |
| 94 | `kleio scan week` | Shows weekly breakdown |

---

## Section 10: Import commands (regression)

| # | Step | Expected |
|---|------|----------|
| 95 | If Cursor transcripts exist: `kleio import cursor` | Imports signals from Cursor transcripts into local SQLite |

---

## Result Summary Template

```
Date:          ____-__-__
Tester:        ____________
Branch:        feature/sync (commit: _______)
Environment:   [staging / local dev]
OS:            [macOS / Linux / Windows]
Go version:    ____________

Section 1 (Local Mode):          __ / 15 passed
Section 2 (Git Indexer):         __ /  9 passed
Section 3 (Trigger Commands):    __ / 15 passed
Section 4 (MCP Local):           __ /  8 passed
Section 5 (Cloud Mode):          __ /  8 passed
Section 6 (Sync):                __ / 11 passed
Section 7 (BYOK LLM):           __ /  8 passed (or N/A)
Section 8 (Edge Cases):          __ / 11 passed
Section 9 (Scan Regression):     __ /  3 passed
Section 10 (Import Regression):  __ /  1 passed

Total:                           __ / 89 passed

Blocking issues:
  1. ____________
  2. ____________

Notes:
  ____________
```
