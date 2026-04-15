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
