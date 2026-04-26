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
dx ask
```

## Commands

| Command | Description |
|---------|-------------|
| `dx ask` | Ask Deductive a question (setup on first use) |
| `dx auth` | Re-authenticate |
| `dx config` | View or change settings |
| `dx upgrade` | Upgrade to the latest version |
| `dx skill install` | Install SKILL.md for AI agent integration |

Run `dx --help` for full details.

## Configuration

Configuration is stored in `~/.dx/` and managed via `dx config`.

```bash
# View current settings
dx config

# Re-run setup wizard (change endpoint or auth)
dx config setup

# Reset all configuration (re-setup on next dx ask)
dx config reset
```

## License

Apache License 2.0 -- see [LICENSE](LICENSE) for details.
