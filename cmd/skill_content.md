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

Running `dx ask` on a fresh install auto-bootstraps: prompts for endpoint URL, then either paste a `dak_...` API key or press Enter for OAuth device-code flow.

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
| `/upload <path>` | Attach a text file to the current session |
| `/new` | Start a fresh session |
| `/resume` | List recent sessions and switch to one |
| `/help` | Show available commands |
| `exit` | End the session |

### dx config

View or change settings.

```bash
# Show current configuration
dx config

# Re-run setup wizard (change endpoint or auth)
dx config setup

# Reset all configuration (re-setup on next dx ask)
dx config reset
```

### dx auth

Re-authenticate (OAuth device flow or API key instructions).

```bash
dx auth
```

### dx upgrade

Upgrade dx to the latest release from GitHub.

```bash
dx upgrade
```

### dx skill

Install or print the dx SKILL.md for AI agent integration.

```bash
# Install SKILL.md to the appropriate agent skill directory
dx skill install

# Overwrite an existing installation
dx skill install --force

# Print the embedded SKILL.md to stdout
dx skill print
```

`dx skill install` auto-detects the agent environment:
- If `.claude/` exists in cwd: writes to `~/.claude/skills/dx/SKILL.md`
- If `.cursor/` exists in cwd: writes to `.cursor/skills/dx/SKILL.md`
- Default: writes to `.cursor/skills/dx/SKILL.md`

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

### Attach a file and ask

```bash
# Pipe a file as context
cat ./thread-dump.txt | dx ask "analyze this thread dump for deadlocks"

# Or in interactive mode, use /upload
dx ask
dx> /upload ./thread-dump.txt
dx> analyze this thread dump for deadlocks
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
