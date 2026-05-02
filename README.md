# dx — CLI for Deductive AI

Ask questions about your infrastructure from the terminal. Get AI-powered insights, investigate incidents, and debug issues — all without leaving your shell.

## Install

### Homebrew (macOS / Linux)

```bash
brew tap deductive-ai/dx
brew install dx
```

### One-line install

```bash
curl -fsSL https://raw.githubusercontent.com/deductive-ai/dx/main/install.sh | bash
```

### GitHub Releases

Download the latest binary for your platform from [Releases](https://github.com/deductive-ai/dx/releases).

### Build from source

```bash
go install github.com/deductive-ai/dx@latest
```

## Quick start

```bash
# Just ask — setup runs automatically on first use
dx ask "what services are unhealthy right now?"

# Pipe data for analysis
kubectl get pods -A | dx ask "which pods are in trouble?"

# Interactive session (multi-turn)
dx
```

### Example session

```
$ dx
Creating session...
Endpoint: https://app.deductive.ai | Session: a1b2c3d4
Type your questions. Use /help for commands. Press Ctrl+D to exit.

dx> what pods are failing in production?
Based on the current cluster state, I found 3 pods in CrashLoopBackOff...

dx> /new
Creating session...
Endpoint: https://app.deductive.ai | Session: e5f6g7h8

dx> what's the p99 latency on the payments service?
```

## Commands

| Command | Description |
|---------|-------------|
| `dx` | Start an interactive session (same as `dx ask`) |
| `dx ask` | Ask Deductive a question (setup on first use) |
| `dx setup` | Configure endpoint, auth, and skills |
| `dx info` | Show status, version, and diagnostics |
| `dx upgrade` | Upgrade to the latest version |
| `dx team` | List and switch teams |

Run `dx --help` for full details.

## Setup & Configuration

Configuration is stored in `~/.dx/` and managed via `dx setup`.

```bash
# Run setup wizard (endpoint + auth)
dx setup init

# Re-authenticate
dx setup auth

# Install agent skill (Cursor, Claude Code, Copilot, Codex)
dx setup skill install

# Reset all configuration
dx setup reset

# View current config
dx setup config
```

## Team Management

```bash
# List teams
dx team

# Switch to a different team
dx team switch <name-or-id>
```

## License

Apache License 2.0 -- see [LICENSE](LICENSE) for details.
