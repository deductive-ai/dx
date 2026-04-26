#!/bin/sh
set -e

REPO="deductive-ai/dx"
BINARY="dx"
INSTALL_DIR="/usr/local/bin"

main() {
    os=$(uname -s | tr '[:upper:]' '[:lower:]')
    arch=$(uname -m)

    case "$arch" in
        x86_64|amd64) arch="amd64" ;;
        aarch64|arm64) arch="arm64" ;;
        *) echo "Unsupported architecture: $arch" >&2; exit 1 ;;
    esac

    case "$os" in
        linux|darwin) ;;
        *) echo "Unsupported OS: $os" >&2; exit 1 ;;
    esac

    echo "Detecting platform: ${os}/${arch}"

    version=$(curl -fsSL "https://api.github.com/repos/${REPO}/releases/latest" \
        | grep '"tag_name"' | head -1 | cut -d'"' -f4)

    if [ -z "$version" ]; then
        echo "Error: could not determine latest version." >&2
        echo "If the repo is private, set GITHUB_TOKEN and retry:" >&2
        echo "  curl -H \"Authorization: token \$GITHUB_TOKEN\" ..." >&2
        exit 1
    fi

    # Strip leading 'v' for the archive name
    ver="${version#v}"
    archive="dx_${ver}_${os}_${arch}.tar.gz"
    url="https://github.com/${REPO}/releases/download/${version}/${archive}"

    echo "Downloading dx ${version} (${os}/${arch})..."

    tmpdir=$(mktemp -d)
    trap 'rm -rf "$tmpdir"' EXIT

    curl -fsSL -o "${tmpdir}/${archive}" "$url"
    tar xzf "${tmpdir}/${archive}" -C "$tmpdir"

    if [ -w "$INSTALL_DIR" ]; then
        mv "${tmpdir}/${BINARY}" "${INSTALL_DIR}/${BINARY}"
    else
        echo "Installing to ${INSTALL_DIR} (requires sudo)..."
        sudo mv "${tmpdir}/${BINARY}" "${INSTALL_DIR}/${BINARY}"
    fi

    chmod +x "${INSTALL_DIR}/${BINARY}"

    echo ""
    echo "dx ${version} installed to ${INSTALL_DIR}/${BINARY}"
    echo ""
    echo "Get started:"
    echo "  dx init    # configure endpoint and authenticate"
    echo "  dx ask     # ask your first question"
}

main
