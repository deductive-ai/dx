# dx CLI

CLI for Deductive AI -- ask questions about your infrastructure from the terminal.

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

For CI / scripts, skip interactive setup entirely with environment variables:

```bash
DX_API_KEY=dak_xxx DX_ENDPOINT=https://acme.deductive.ai dx ask "status?"
```

## Commands

### dx ask [question]

Primary command. Sends a question to Deductive and streams the answer.

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

**Piped input:** when stdin is not a TTY, dx reads it, uploads as context, then opens `/dev/tty` for interactive follow-ups (or runs one-shot if a question argument is provided).

### dx profile

Manage named profiles (each with its own endpoint, auth, sessions).

```bash
# List profiles (* marks active)
dx profile

# Switch active profile
dx profile use staging

# Delete a profile
dx profile delete old-env
```

**Profile precedence:** `--profile` flag > `DX_PROFILE` env var > `~/.dx/active_profile` file > `"default"`

### dx config

Configure endpoint and authentication for the current profile.

```bash
# Interactive setup
dx config

# Non-interactive
dx config --endpoint=https://acme.deductive.ai --api-key=dak_xxxxx

# OAuth mode
dx config --endpoint=https://acme.deductive.ai --auth-mode=oauth

# List all profiles
dx config list

# Delete a profile
dx config delete --profile=old
```

**Flags:**
- `--endpoint`, `-e` -- Deductive endpoint URL
- `--api-key` -- API key (implies apikey auth mode)
- `--auth-mode` -- `oauth` or `apikey`
- `--no-validate` -- skip endpoint connectivity check

### dx upload

Upload files or stdin as context for the current session.

```bash
# Upload a file
dx upload -f /path/to/logs.txt

# Upload a directory recursively
dx upload -f ./configs -r

# Upload from stdin
some-command | dx upload --stdin --name output.txt
```

**Flags:**
- `-f`, `--file` -- file or directory path
- `-r`, `--recursive` -- upload directory contents
- `--stdin` -- read from stdin
- `--name` -- filename for stdin content (default: `stdin.txt`)

### dx hook

Register shell scripts that run before each `dx ask` message, appending their output as context.

```bash
# Add a hook
dx hook add ./gather-context.sh

# List registered hooks
dx hook list

# Remove a hook by index
dx hook remove 0
```

Hooks run with a 30-second timeout. Output is included as an `<appendix>` in the message only when it changes between messages.

### dx session

Manage conversation sessions.

```bash
# List sessions for current profile
dx session list

# Delete a specific session
dx session delete <id>

# Clear all sessions for current profile
dx session clear
```

### dx auth

Re-authenticate the current profile (OAuth device flow or API key refresh).

```bash
dx auth
```

### dx upgrade

Upgrade dx to the latest release from GitHub.

```bash
dx upgrade
```

### dx status

Show current profile, endpoint, auth state, and session info.

```bash
dx status
dx status --json
```

## Global flags

These work on every command:

- `--profile` -- select a named profile (overrides `DX_PROFILE` and active profile)
- `--no-color` -- disable color output (also `NO_COLOR` or `DX_NO_COLOR` env vars)
- `--debug` -- enable debug logging (also `DX_DEBUG` env var)

## Environment variables

| Variable | Purpose |
|----------|---------|
| `DX_API_KEY` | API key for non-interactive auth (CI/scripts) |
| `DX_ENDPOINT` | Endpoint URL for non-interactive auth |
| `DX_PROFILE` | Override active profile name |
| `DX_DEBUG` | Enable debug logging |
| `DX_NO_COLOR` / `NO_COLOR` | Disable color output |

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

### Multi-environment investigation

```bash
dx ask --profile=production "is the API healthy?"
dx ask --profile=staging "compare latency to production"
```

### CI / automation

```bash
export DX_API_KEY=dak_xxx
export DX_ENDPOINT=https://acme.deductive.ai
dx ask "nightly health check -- any anomalies in the last 24h?"
```

### Attach files then ask

```bash
dx upload -f ./thread-dump.txt
dx ask "analyze this thread dump for deadlocks"
```

## File layout

```
~/.dx/
  active_profile          # name of the active profile
  history                 # readline history for interactive mode
  version_cache.json      # cached version check (1h TTL)
  profiles/
    <profile>/
      config              # TOML: endpoint, auth_mode, tokens, hooks
      current_session     # pointer to active session ID
  sessions/
    <session-id>          # JSON: session state (encrypted URLs)
```
