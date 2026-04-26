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

> **Private repo?** While the repo is private, use a GitHub token:
> ```bash
> curl -fsSL -H "Authorization: token $GITHUB_TOKEN" \
>   https://raw.githubusercontent.com/deductive-ai/dx/main/install.sh | bash
> ```

### GitHub Releases

Download the latest binary for your platform from [Releases](https://github.com/deductive-ai/dx/releases).

### Build from source

```bash
go install github.com/deductive-ai/dx@latest
```

> While the repo is private, set `GOPRIVATE=github.com/deductive-ai/*` first.

## Quick start

```bash
# One-command setup — configure endpoint, authenticate, install completions
dx init

# Ask a question
dx ask "what services are unhealthy right now?"

# Pipe data for analysis
kubectl get pods -A | dx ask "which pods are in trouble?"

# Upload files for context
dx upload -f /tmp/incident.log
dx ask "what caused the errors in the uploaded log?"
```

## Commands

| Command | Description |
|---------|-------------|
| `dx init` | First-time setup wizard |
| `dx ask` | Ask Deductive a question |
| `dx login` | Re-authenticate |
| `dx status` | Connection and session status |
| `dx upload` | Upload files for context |
| `dx session list` | List sessions |
| `dx version` | Print version |

Run `dx --help` for full details.

## Configuration

Configuration is stored in `~/.dx/profiles/<profile>/config` (TOML format).

```bash
# Set up a second profile (e.g. staging)
dx init --profile=staging

# Use a specific profile
dx ask "test query" --profile=staging
```

## License

Copyright (c) 2025 Deductive AI, Inc. All rights reserved.
