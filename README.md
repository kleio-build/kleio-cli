# kleio-cli

CLI and MCP server for Kleio — capture work discovered during development.

## Install

### Quick install (recommended)

**macOS / Linux:**

```bash
curl -fsSL https://raw.githubusercontent.com/kleio-build/kleio-cli/main/install.sh | sh
```

**Windows (PowerShell):**

```powershell
irm https://raw.githubusercontent.com/kleio-build/kleio-cli/main/install.ps1 | iex
```

### Homebrew (macOS / Linux)

```bash
brew install kleio-build/tap/kleio
```

Or:

```bash
brew tap kleio-build/tap
brew install kleio
```

### Scoop (Windows)

```powershell
scoop bucket add kleio https://github.com/kleio-build/scoop-bucket
scoop install kleio
```

### APT (Debian / Ubuntu)

Download the `.deb` package from [GitHub Releases](https://github.com/kleio-build/kleio-cli/releases) and install:

```bash
curl -LO https://github.com/kleio-build/kleio-cli/releases/latest/download/kleio_<version>_linux_amd64.deb
sudo dpkg -i kleio_<version>_linux_amd64.deb
```

### RPM (Fedora / RHEL / CentOS)

Download the `.rpm` package from [GitHub Releases](https://github.com/kleio-build/kleio-cli/releases) and install:

```bash
curl -LO https://github.com/kleio-build/kleio-cli/releases/latest/download/kleio_<version>_linux_amd64.rpm
sudo rpm -i kleio_<version>_linux_amd64.rpm
```

### Go install

```bash
go install github.com/kleio-build/kleio-cli/cmd/kleio@latest
```

### Manual download

Download the latest release from [GitHub Releases](https://github.com/kleio-build/kleio-cli/releases).

## Updating

| Method | Update command |
|--------|----------------|
| Quick install | Re-run the install script — it fetches the latest release |
| Go install | `go install github.com/kleio-build/kleio-cli/cmd/kleio@latest` |
| Homebrew | `brew upgrade kleio` |
| Scoop | `scoop update kleio` |
| APT (deb) | Download the new `.deb` from [Releases](https://github.com/kleio-build/kleio-cli/releases) and `sudo dpkg -i kleio_*.deb` |
| RPM | Download the new `.rpm` from [Releases](https://github.com/kleio-build/kleio-cli/releases) and `sudo rpm -U kleio_*.rpm` |
| Manual | Download the new binary from [Releases](https://github.com/kleio-build/kleio-cli/releases) |

```bash
kleio --version
```

## Project bootstrap (`kleio init`)

Templates (AGENTS.md, Cursor rules, Claude stub, skills, config examples) ship inside the CLI. From your repo root:

```bash
# Default: install recommended profile for this repo (often Cursor-oriented)
kleio init

# Interactive wizard: tooling, embedded GitHub sign-in + workspace selection, init verify
kleio init -i

# Profiles: cursor, claude, generic (AGENTS.md only), none (skip files), all (cursor+claude)
kleio init --tool=cursor
kleio init --tool=cursor,claude

# CI / automation: no prompts; pass --tool when the repo has no .cursor/.claude/ signals
kleio init --non-interactive --yes-new-only --tool=cursor

# When a file already exists, init writes a sidecar (e.g. AGENTS.kleio.md, .cursor/mcp.kleio.json.example)
# unless you confirm overwrite interactively or pass --force-overwrite
```

After a successful **Init verify**, the CLI prints a one-liner `kleio checkpoint ...` so you can confirm API access. Configure MCP using `.cursor/mcp.json.example` (merge into your Cursor MCP config).

```bash
kleio config set api-url <url>        # optional
kleio config set workspace-id <uuid>  # or pick during kleio login / kleio init -i
kleio config show                     # secrets are redacted
```

### API environment

With no config file, the CLI targets **production** (`https://api.kleio.build`). Switch presets with:

```bash
kleio config use production   # default API host
kleio config use staging      # https://api.dev.kleio.build
kleio config use local        # http://localhost:8080 + dev API key for local stack
```

Each environment keeps its own credentials in `~/.kleio/environments/<env>.yaml`, so switching is instant — no re-login required after the first `kleio login` per environment. Override for one-off commands: `KLEIO_ENV=staging` or `KLEIO_API_URL=https://...`.

## Configuration check (`kleio status`)

Validates local config, API health, authentication, and workspace-scoped **counts** (`GET /api/workspace/counts`). Prints an informational **MCP** section (never fails the command). Use `--json` to print only the raw workspace counts JSON.

```bash
kleio status
kleio status --json
```

## Workspace resolution

The CLI and MCP server resolve the active workspace using a layered strategy:

| Priority | Source | Description |
|----------|--------|-------------|
| 1 | `KLEIO_WORKSPACE_ID` env var | Explicit override, always wins |
| 2 | Git remote auto-detection | Parses the origin remote URL from `.git/config`, extracts the GitHub owner (e.g. `kleio-build`), and matches it against your workspaces |
| 3 | Project `.kleio/config.yaml` | Walks up from the current directory looking for `.kleio/config.yaml` with a `workspace_id` field |
| 4 | `~/.kleio/config.yaml` | Home config set by `kleio login` or `kleio workspace select` |

This means when you `cd` into a `kleio-build/*` repo, captures automatically go to the kleio-build workspace. Switch to a `kalo-build/*` repo and they go to kalo-build -- no manual workspace switching needed.

### Project-level config

For repos where git remote detection doesn't apply (non-GitHub remotes, forks, monorepos), place a `.kleio/config.yaml` in the project root:

```yaml
workspace_id: "d1234567-..."
```

Run `kleio status` to verify which source was used:

```
Workspace: d1234567-... (auto-detected from git remote (owner: kleio-build))
```

The MCP server resolves the workspace once at startup based on the editor's working directory.

## CLI usage

```bash
# Configure (defaults target production; use local stack:)
kleio config use local
# or: kleio config set api-url http://localhost:8080
kleio config set api-key your-api-key

# Capture work items
kleio capture "add retry logic to billing webhook"
kleio capture "auth middleware is duplicated" --file auth.go --line 88 --tag tech-debt
kleio capture "needs migration rollback path" --repo api-server --context "found during deploy"

# Review backlog
kleio backlog list
kleio backlog list --status new --priority high
kleio backlog show <id>

# Prioritize
kleio backlog prioritize <id> --priority high --status ready

# JSON output for scripts/agents
kleio capture "task" --json
kleio backlog list --json
```

## Reports & Output Formats

The `trace`, `explain`, and `incident` commands produce structured reports with sections for **About**, **Decisions**, **Open Threads**, **Code Changes**, **Evidence Quality**, and **Next Steps**. When Ollama is running locally (auto-detected at `localhost:11434`) or a BYOK provider is configured, the report is enriched with LLM-generated prose and semantic deduplication.

```bash
kleio trace "auth" --format text       # default: human-readable plain text
kleio trace "auth" --format md         # Markdown with tables and headers
kleio trace "auth" --format html -o report.html   # standalone HTML
kleio trace "auth" --format pdf -o report.pdf      # PDF via go-pdf/fpdf
kleio trace "auth" --format json       # machine-readable JSON
kleio trace "auth" --verbose           # append raw timeline to output
kleio trace "auth" --no-llm            # skip LLM enrichment
```

The same flags work for `kleio explain` and `kleio incident`.

By default reports filter to the **current repository** (detected from the git remote). Pass `--all-repos` to widen any of `trace`, `explain`, or `incident` across every indexed repo:

```bash
kleio trace "og" --since 60d                # current repo only
kleio trace "og" --since 60d --all-repos    # cross-repo retrieval
```

## Ingest pipeline

`kleio ingest` runs a four-stage local-first pipeline that turns the artifacts you already produce — Cursor plans, Cursor transcripts, git history — into a queryable corpus of `Event` rows.

```
sources --> Ingester --> RawSignal --> Correlator --> Cluster --> Synthesizer --> Event
                                                                                  |
                                                                                  v
                                                                              SQLite
```

| Stage | Implementations |
|-------|------------------|
| **Ingest**      | `plan` (`.cursor/plans/*.plan.md` multi-pass parser), `transcript` (narrow-accept Cursor exports), `git` (commits via go-git) |
| **Correlate**   | `time_window`, `id_reference` (KL-N / PR-# / plan hashes / SHAs), `file_path`, `search` (FTS5 locally / embeddings on cloud), `embed` (auto-promoted when an LLM is available) |
| **Synthesize**  | `plan_cluster` (umbrella + todos + decisions + deferred children), `orphan` (only explicit pass-throughs), `llm` (optional summary refinement) |

```bash
kleio ingest                          # all sources, current repo
kleio ingest --source plan            # plans only
kleio ingest --source transcript,git
kleio ingest --all-repos              # cross-repo
kleio ingest --since 30d              # incremental
kleio ingest --no-llm                 # disable EmbedCorrelator + LLMSynthesizer
kleio ingest --reimport               # wipe synthesized signals first
kleio ingest --dry-run                # preview without writing
```

`kleio import cursor` continues to work and aliases to plan + transcript ingestion for back-compat.

For per-stage debugging, hidden dev commands print their output without persisting:

```bash
kleio dev ingest plan       # one ingester at a time
kleio dev ingest all
kleio dev correlate         # full ingest + every correlator
kleio dev synthesize        # full ingest + correlate + every synthesizer
kleio dev smoke-report      # render trace/explain/incident reports in every format with PDF read-back validation
```

### Anchor aliases

`trace`, `explain`, and `incident` widen the search anchor through `~/.kleio/aliases.yaml`. Each alias is OR'd into the FTS5 query and commit-message LIKE matcher.

```yaml
aliases:
  og: [opengraph, og-image, "og:image", twitter:card]
  auth: [authentication, login, jwt, session]
  pr: [pull request, code review, review request]
```

When an `ai.Provider` (Ollama or BYOK) is available, the expander additionally asks the model for 3-5 short related terms and unions them with the static set. Responses are cached at `~/.kleio/alias-cache.json` for 30 days.

### Multi-repo workspaces

By default every command scopes to the current repository — detected from the git remote in the working directory. This is deliberate: running `kleio trace "auth"` inside the `kleio-cli` repo surfaces only `kleio-cli` signals, matching the scope established at ingest time.

Pass `--all-repos` to widen scope across every indexed repo:

```bash
kleio ingest --all-repos             # ingest plans + transcripts + git from all projects
kleio trace "auth" --all-repos       # cross-repo retrieval
kleio explain main.go --all-repos    # cross-repo file history
kleio incident --all-repos           # cross-repo incident view
```

Scope is determined by `CursorScope.Mode` (configured via `.kleio/config.yaml` or auto-detected). When `--all-repos` is absent the CLI calls `currentRepoName()` and passes it as a filter on every `Store.QueryCommits` and `Store.ListEvents` call.

### Adding a new ingest source

A source is anything that produces `kleio.RawSignal`s. Sketch for a Slack ingester:

1. Implement `kleio.Ingester` in `internal/ingest/slack/ingester.go`. `Ingest(ctx, scope) ([]RawSignal, error)` is the only required method. Each Slack message becomes one `RawSignal` with `SourceType = "slack"`, `SourceID = <channel>:<ts>`, `Content = message.text`, and `Metadata = {channel, thread_ts, user, permalink}`.
2. Register the ingester in `internal/pipeline/builder.go` `Build`. Wire any source-specific config (workspace token, channel allow-list) through the `Config` struct.
3. Add scope discovery to `internal/ingest/discovery/discovery.go` if the source needs to know which Slack workspaces / channels to walk.
4. Optionally extend `internal/correlate/idreference/correlator.go` to recognize Slack permalinks so commits / plans referencing a thread can join the same cluster.

The same template applies to Linear, GitHub PRs, Notion, etc. Every ingester gets free time-window, file-path, ID-reference, and semantic correlation — synthesis layers do not need to know which source the signal came from.

## MCP (Cursor and other editors)

Use the **`kleio` binary** with the `mcp` subcommand (stdio transport). Add to `.cursor/mcp.json`:

```json
{
  "mcpServers": {
    "kleio": {
      "command": "kleio",
      "args": ["mcp"],
      "env": {
        "KLEIO_ENV": "local"
      }
    }
  }
}
```

With no `env` block, the CLI defaults to production. For a custom host only, you can set `KLEIO_API_URL` / `KLEIO_API_KEY` instead. The `kleio mcp` process reloads `~/.kleio/config.yaml` about every 30s (credentials and API URL).

### Available tools

- `kleio_capture` — capture a work item with context
- `kleio_backlog_list` — list backlog items with filters
- `kleio_backlog_show` — show backlog item details
- `kleio_backlog_prioritize` — update priority/status

## Releasing new versions

Releases are automated with [GoReleaser](https://goreleaser.com/) via GitHub Actions. Create and push a version tag:

```bash
git tag v1.2.3
git push origin v1.2.3
```

The pipeline builds binaries for Linux, macOS, and Windows (amd64/arm64 where applicable), publishes a GitHub Release with `.deb` and `.rpm` packages for Linux, and updates the [Homebrew tap](https://github.com/kleio-build/homebrew-tap) and [Scoop bucket](https://github.com/kleio-build/scoop-bucket).

**Secrets** on `kleio-build/kleio-cli`: `HOMEBREW_TAP_GITHUB_TOKEN` and `SCOOP_BUCKET_GITHUB_TOKEN` (PATs with push access to those repos).

### Version format

Use [semantic versioning](https://semver.org/): `v1.0.0`, `v1.1.0`, `v1.1.1` for stable releases.

### Prereleases

Tags with prerelease suffixes are marked as prereleases on GitHub. Prereleases are not pushed to Homebrew or Scoop (stable releases only). To test a prerelease, download the binary from the [Releases](https://github.com/kleio-build/kleio-cli/releases) page.

## License

[MIT License](LICENSE)
