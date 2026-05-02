# dx CLI

CLI for Deductive AI -- ask questions about your infrastructure from the terminal.

Use when the user asks to investigate, debug, query, or monitor infrastructure --
health checks, error analysis, log inspection, resource usage, deployment status,
or any operational question about services, pods, databases, or cloud resources.
Also use when piping CLI output (kubectl, docker, ps, netstat, etc.) for AI analysis.

## Install

```bash
# Homebrew
brew tap deductive-ai/dx && brew install dx

# One-liner
curl -fsSL https://raw.githubusercontent.com/deductive-ai/dx/main/install.sh | bash

# Go
go install github.com/deductive-ai/dx@latest

# Self-update (once installed)
dx upgrade
```

## First-time setup

Run `dx setup init` to configure the CLI: prompts for endpoint URL, then either paste a `dak_...` API key or press Enter for OAuth device-code flow. Running `dx ask` on a fresh install also auto-bootstraps.

## Commands

### dx ask [question]

Primary command. Sends a question to Deductive and streams the answer. Running bare `dx` with no subcommand is equivalent to `dx ask` (interactive mode).

```bash
# One-shot question
dx ask "what services are unhealthy?"

# Pipe data as context
kubectl get pods -A | dx ask "which pods need attention?"
cat error.log | dx ask "what caused this failure?"

# Interactive multi-turn session (no args)
dx ask

# Force a fresh session (ignore existing)
dx ask --new "start from scratch"

# Resume a specific session by ID (or short prefix)
dx ask --session abc123 "follow-up question"

# Timeout after N seconds (0 = no limit)
dx ask --timeout 60 "long analysis"
```

**Flags:**
- `--new` -- force a new session instead of reusing the current one
- `--session`, `-s` -- resume a specific session by ID
- `--timeout` -- max seconds to wait for a complete response (default: 0 / unlimited)

**Session behavior:** sessions auto-expire after 30 minutes of inactivity. When reusing an existing session, dx prints "Continuing session (X ago)". Use `--new` to start fresh.

**Piped input:** when stdin is not a TTY, dx reads it as additional context for the question.

**Interactive commands (inside `dx ask`):**

| Command | Description |
|---------|-------------|
| `/new` | Start a fresh session |
| `/resume` | List recent sessions and switch to one |
| `/help` | Show available commands |
| `exit` | End the session |

### dx setup

Configure endpoint, auth, and skills. Bare `dx setup` shows available subcommands.

```bash
# Run setup wizard (endpoint + auth)
dx setup init

# Re-authenticate
dx setup auth

# Install SKILL.md to agent skill directories
dx setup skill install

# Overwrite an existing skill installation
dx setup skill install --force

# Print the embedded SKILL.md to stdout
dx setup skill print

# Reset all configuration (re-setup with dx setup init)
dx setup reset

# View current config
dx setup config
```

`dx setup skill install` writes to all user-level agent skill directories:
- `~/.claude/skills/dx/SKILL.md` (Claude Code)
- `~/.cursor/skills/dx/SKILL.md` (Cursor)
- `~/.copilot/skills/dx/SKILL.md` (GitHub Copilot)
- `~/.agents/skills/dx/SKILL.md` (OpenAI Codex)

One install covers every agent in every project.

### dx info

Show current CLI status, team, session, and version.

```bash
# Human-readable status
dx info

# JSON output for scripting
dx info --json

# Version only
dx info version
```

### dx upgrade

Upgrade dx to the latest release from GitHub.

```bash
dx upgrade
```

### dx team

List and switch teams.

```bash
# List teams (marks active with *)
dx team

# Switch to a different team
dx team switch <name-or-id>
```

## Global flags

- `--no-color` -- disable color output (also `NO_COLOR` or `DX_NO_COLOR` env vars)
- `--debug` -- enable debug logging (also `DX_DEBUG` env var)

## Key workflows

### One-shot question from an agent

```bash
dx ask "what's the error rate on the payments service?"
```

### Pipe context and ask

```bash
kubectl logs deploy/api --tail=100 | dx ask "summarize errors"
docker stats --no-stream | dx ask "which containers need attention?"
```

### Pipe a file as context

```bash
cat ./thread-dump.txt | dx ask "analyze this thread dump for deadlocks"
```

## File layout

```
~/.dx/
  active_profile          # internal: active configuration name
  history                 # readline history for interactive mode
  version_cache.json      # cached version check (1h TTL)
  profiles/
    default/
      config              # TOML: endpoint, auth tokens
      current_session     # pointer to active session ID
  sessions/
    <session-id>          # JSON: session state
```
