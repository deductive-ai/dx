# Contributing to dx

Thanks for your interest in contributing to the Deductive CLI.

## Development

### Prerequisites

- Go 1.22+
- A Deductive instance to test against

### Build

```bash
go build -o dx .
```

### Test

```bash
go test ./...
```

### Lint

```bash
# Install golangci-lint: https://golangci-lint.run/welcome/install/
golangci-lint run
```

## Submitting changes

1. Fork the repo and create a feature branch
2. Make your changes
3. Ensure `go test ./...` passes
4. Ensure `go build .` succeeds
5. Submit a pull request

## Reporting issues

Open an issue on GitHub with:
- The output of `dx version`
- Steps to reproduce
- Expected vs actual behavior
