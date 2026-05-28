#!/usr/bin/env sh
# yap install script — fetch the latest release binary for the current
# OS/architecture from GitHub and drop it in $YAP_INSTALL_DIR (default
# /usr/local/bin, or $HOME/.local/bin when the prefix is not writable).
#
# Usage:
#   curl -fsSL https://raw.githubusercontent.com/M0Rf30/yap/main/scripts/install.sh | sh
#   # pinned version:
#   curl -fsSL https://raw.githubusercontent.com/M0Rf30/yap/main/scripts/install.sh | sh -s -- --version v2.1.3
#   # install yap-mcp instead of yap (default installs both):
#   curl -fsSL .../install.sh | sh -s -- --tool yap-mcp
#   # alternative install dir:
#   curl -fsSL .../install.sh | YAP_INSTALL_DIR=$HOME/bin sh
#
# Supported OS/arch combinations match the goreleaser matrix:
#   linux/amd64, linux/arm64, darwin/amd64, darwin/arm64
set -eu

REPO="${YAP_REPO:-M0Rf30/yap}"
GITHUB_API="https://api.github.com/repos/${REPO}/releases"
GITHUB_DL="https://github.com/${REPO}/releases/download"

# ---------- helpers ---------------------------------------------------

log()  { printf '\033[1;34m▸\033[0m %s\n' "$*" >&2; }
warn() { printf '\033[1;33m▸ warn:\033[0m %s\n' "$*" >&2; }
err()  { printf '\033[1;31m✖\033[0m %s\n' "$*" >&2; }
die()  { err "$*"; exit 1; }

need() {
    command -v "$1" >/dev/null 2>&1 || die "required command not found: $1"
}

# ---------- arg parsing -----------------------------------------------

VERSION=""
TOOLS="yap yap-mcp"

while [ $# -gt 0 ]; do
    case "$1" in
        --version|-v)  VERSION="$2"; shift 2 ;;
        --version=*)   VERSION="${1#*=}"; shift ;;
        --tool|-t)     TOOLS="$2"; shift 2 ;;
        --tool=*)      TOOLS="${1#*=}"; shift ;;
        --help|-h)
            sed -n '2,18p' "$0" 2>/dev/null || cat <<EOF
yap install script. Flags:
  --version <tag>   Pin a release tag (default: latest).
  --tool <name>     "yap", "yap-mcp", or both (default: both).
  YAP_INSTALL_DIR   Install prefix (default: /usr/local/bin or ~/.local/bin).
  YAP_REPO          GitHub owner/repo (default: M0Rf30/yap).
EOF
            exit 0 ;;
        *) die "unknown argument: $1" ;;
    esac
done

# ---------- prereqs ---------------------------------------------------

need uname
need tar
need rm
need mkdir
need mv

SHASUM=""
if command -v sha256sum >/dev/null 2>&1; then
    SHASUM="sha256sum"
elif command -v shasum >/dev/null 2>&1; then
    SHASUM="shasum -a 256"
fi

DL=""
if command -v curl >/dev/null 2>&1; then
    DL="curl"
elif command -v wget >/dev/null 2>&1; then
    DL="wget"
else
    die "need curl or wget to download release archives"
fi

fetch() {
    # fetch URL outfile  → writes to outfile, fails on HTTP error
    if [ "$DL" = "curl" ]; then
        curl -fsSL "$1" -o "$2"
    else
        wget -q -O "$2" "$1"
    fi
}

fetch_stdout() {
    if [ "$DL" = "curl" ]; then
        curl -fsSL "$1"
    else
        wget -q -O - "$1"
    fi
}

# ---------- detect OS/arch -------------------------------------------

uname_s="$(uname -s)"
uname_m="$(uname -m)"

case "$uname_s" in
    Linux)  OS="Linux" ;;
    Darwin) OS="Darwin" ;;
    *) die "unsupported OS: $uname_s (only Linux and Darwin builds are published)" ;;
esac

case "$uname_m" in
    x86_64|amd64) ARCH="x86_64" ;;
    aarch64|arm64) ARCH="arm64" ;;
    *) die "unsupported architecture: $uname_m (need x86_64 or arm64)" ;;
esac

ARCHIVE_SUFFIX="${OS}_${ARCH}.tar.gz"

# ---------- resolve version ------------------------------------------

if [ -z "$VERSION" ]; then
    log "resolving latest release tag for $REPO ..."
    # /releases/latest returns a tag like v2.1.3
    if ! VERSION="$(fetch_stdout "${GITHUB_API}/latest" \
        | sed -n 's/.*"tag_name": *"\([^"]*\)".*/\1/p' | head -n1)"; then
        die "failed to query latest release"
    fi
    [ -n "$VERSION" ] || die "could not parse latest tag from GitHub API"
fi

case "$VERSION" in
    v*) VERSION_TRIMMED="${VERSION#v}" ;;
    *)  VERSION_TRIMMED="$VERSION"; VERSION="v$VERSION" ;;
esac

log "installing version $VERSION ($OS/$ARCH)"

# ---------- pick install dir -----------------------------------------

choose_install_dir() {
    if [ -n "${YAP_INSTALL_DIR:-}" ]; then
        printf '%s' "$YAP_INSTALL_DIR"
        return
    fi

    for d in /usr/local/bin /opt/homebrew/bin; do
        if [ -w "$d" ] 2>/dev/null; then
            printf '%s' "$d"
            return
        fi
    done

    # No writable system dir — fall back to per-user dir.
    printf '%s' "$HOME/.local/bin"
}

INSTALL_DIR="$(choose_install_dir)"
mkdir -p "$INSTALL_DIR" || die "cannot create install dir: $INSTALL_DIR"
[ -w "$INSTALL_DIR" ] || die "install dir is not writable: $INSTALL_DIR (set YAP_INSTALL_DIR or rerun with sudo)"

case ":$PATH:" in
    *":$INSTALL_DIR:"*) ;;
    *) warn "$INSTALL_DIR is not on your PATH — add it to your shell rc (e.g. export PATH=\"$INSTALL_DIR:\$PATH\")" ;;
esac

# ---------- install one tool -----------------------------------------

install_tool() {
    name="$1"
    archive="${name}_${ARCHIVE_SUFFIX}"
    url="${GITHUB_DL}/${VERSION}/${archive}"

    log "downloading $archive"
    tmpdir="$(mktemp -d -t yap-install.XXXXXX)"
    # shellcheck disable=SC2064
    trap "rm -rf '$tmpdir'" EXIT INT TERM

    if ! fetch "$url" "$tmpdir/$archive"; then
        die "failed to download $url"
    fi

    # Verify SHA256 against the release's checksums.txt when we have a hasher.
    if [ -n "$SHASUM" ]; then
        checksums_url="${GITHUB_DL}/${VERSION}/checksums.txt"
        if fetch "$checksums_url" "$tmpdir/checksums.txt" 2>/dev/null; then
            expected="$(awk -v a="$archive" '$2 == a { print $1 }' "$tmpdir/checksums.txt")"
            if [ -n "$expected" ]; then
                actual="$(cd "$tmpdir" && $SHASUM "$archive" | awk '{print $1}')"
                if [ "$expected" != "$actual" ]; then
                    die "checksum mismatch for $archive (expected $expected, got $actual)"
                fi
                log "sha256 ok"
            else
                warn "no checksum entry for $archive in checksums.txt — skipping verification"
            fi
        else
            warn "could not fetch checksums.txt — skipping verification"
        fi
    else
        warn "no sha256sum/shasum available — skipping checksum verification"
    fi

    log "extracting"
    ( cd "$tmpdir" && tar -xzf "$archive" )

    bin="$tmpdir/$name"
    [ -f "$bin" ] || die "archive did not contain a '$name' binary"

    chmod +x "$bin"

    dst="$INSTALL_DIR/$name"
    if [ -e "$dst" ] && [ ! -w "$dst" ]; then
        die "$dst exists and is not writable — rerun with sudo or set YAP_INSTALL_DIR"
    fi

    mv "$bin" "$dst"
    log "installed $dst"

    # Best-effort version print. yap-mcp speaks JSON-RPC on stdin and has no
    # CLI flag surface, so calling it with --version would hang waiting for a
    # client — skip the post-install version probe for it.
    if [ "$name" = "yap" ] && "$dst" --version </dev/null >/dev/null 2>&1; then
        "$dst" --version </dev/null 2>/dev/null | head -n1 | sed 's/^/    /'
    fi

    rm -rf "$tmpdir"
    trap - EXIT INT TERM
}

for tool in $TOOLS; do
    case "$tool" in
        yap|yap-mcp) install_tool "$tool" ;;
        *) die "unknown tool: $tool (want yap or yap-mcp)" ;;
    esac
done

log "done. yap $VERSION_TRIMMED ready in $INSTALL_DIR"
