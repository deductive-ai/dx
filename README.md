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

# For CI / scripts, use environment variables
DX_API_KEY=dak_xxx DX_ENDPOINT=https://acme.deductive.ai dx ask "status?"
```

## Commands

| Command | Description |
|---------|-------------|
| `dx ask` | Ask Deductive a question (setup on first use) |
| `dx auth` | Re-authenticate |
| `dx profile` | List and manage profiles |
| `dx upgrade` | Upgrade to the latest version |

Run `dx --help` for full details.

## Configuration

Configuration is stored in `~/.dx/profiles/<profile>/config` (TOML format).

```bash
# Use a specific profile
dx ask "test query" --profile=staging
```

## License

Apache License 2.0 -- see [LICENSE](LICENSE) for details.
