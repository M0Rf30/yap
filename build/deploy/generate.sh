#!/usr/bin/env bash
# generate.sh — regenerate all per-distro Dockerfiles from templates.
# Usage: bash build/deploy/generate.sh
# Requires: bash 4+, no external deps.
set -euo pipefail

SCRIPT_DIR="$(cd "$(dirname "${BASH_SOURCE[0]}")" && pwd)"
REPO_ROOT="$(cd "${SCRIPT_DIR}/../.." && pwd)"

# ── Shared builder stage ──────────────────────────────────────────────────────
builder_stage() {
  cat <<'BUILDER'
# syntax=docker/dockerfile:1
ARG GO_VERSION=1.26.1

# Build stage
FROM golang:${GO_VERSION}-alpine AS builder

# Build arguments for metadata
ARG VERSION=dev
ARG COMMIT=unknown
ARG BUILD_TIME=unknown
ARG TARGETARCH=amd64

# Install build dependencies
RUN apk add --no-cache upx git

# Set up build environment
WORKDIR /build
COPY go.mod go.sum ./
RUN go mod download

# Copy source code
COPY . .

# Build optimized binary
RUN CGO_ENABLED=0 GOOS=linux GOARCH=${TARGETARCH} \
    go build \
    -buildvcs=false \
    -ldflags "-w -s -X github.com/M0Rf30/yap/v2/pkg/buildinfo.Version=${VERSION} -X github.com/M0Rf30/yap/v2/pkg/buildinfo.Commit=${COMMIT} -X github.com/M0Rf30/yap/v2/pkg/buildinfo.BuildTime=${BUILD_TIME}" \
    -o /usr/bin/yap \
    ./cmd/yap && \
    upx --best --lzma /usr/bin/yap

# Generate bash completion (do this in build stage where we can execute the binary)
RUN if [ "${TARGETARCH}" = "$(go env GOARCH)" ]; then \
    /usr/bin/yap completion bash > /tmp/yap-completion.bash; \
    else \
    echo "# Cross-compilation: bash completion will be generated at runtime" > /tmp/yap-completion.bash; \
    fi

BUILDER
}

# ── Shared runtime footer ─────────────────────────────────────────────────────
runtime_footer() {
  cat <<'FOOTER'
# Copy binary and completion from builder
COPY --from=builder /usr/bin/yap /usr/bin/yap
COPY --from=builder /tmp/yap-completion.bash /usr/share/bash-completion/completions/yap

# Set up bash completion
RUN echo "source /usr/share/bash-completion/bash_completion" >> /etc/bash.bashrc

# Switch to non-root user
USER yap

ENTRYPOINT ["yap"]
FOOTER
}

# ── Runtime header (FROM + labels) ────────────────────────────────────────────
runtime_header() {
  local base_image="$1" title="$2" description="$3"
  cat <<HEADER
# Runtime stage
FROM ${base_image}

# Build arguments for runtime stage
ARG VERSION=dev
ARG TARGETARCH=amd64
ARG GO_VERSION=1.26.1

# Metadata labels
LABEL org.opencontainers.image.title="${title}"
LABEL org.opencontainers.image.description="${description}"
LABEL org.opencontainers.image.vendor="M0Rf30"
LABEL org.opencontainers.image.source="https://github.com/M0Rf30/yap"
LABEL org.opencontainers.image.licenses="GPL-3.0"
LABEL org.opencontainers.image.version="\${VERSION}"

HEADER
}

# ── Package manager blocks ────────────────────────────────────────────────────
block_apt() {
  local sudoers="$1"
  cat <<APT
# Use bash with pipefail for safer pipes in RUN instructions (hadolint DL4006)
SHELL ["/bin/bash", "-o", "pipefail", "-c"]

# Environment variables
ENV DEBIAN_FRONTEND=noninteractive

# Preseed debconf to prevent resolvconf postinst failure in containers
RUN echo "resolvconf resolvconf/linkify-resolvconf boolean false" | debconf-set-selections

# Install minimal runtime dependencies
RUN apt-get update && \\
    apt-get upgrade -y && \\
    apt-get install -y --no-install-recommends \\
    bash-completion \\
    binutils \\
    ca-certificates \\
    ccache \\
    sudo && \\
    # Set timezone
    ln -sf /usr/share/zoneinfo/UTC /etc/localtime && \\
    # Clean up
    apt-get autoremove -y && \\
    apt-get clean && \\
    rm -rf /var/lib/apt/lists/*

# Make ccache visible to every compiler invocation. The Debian/Ubuntu ccache
# package installs compiler symlinks under /usr/lib/ccache; placing that
# directory first in PATH wraps gcc/g++ and cross-compilers transparently.
ENV PATH="/usr/lib/ccache:\${PATH}"

# Remove default ubuntu user and create yap user at uid/gid 1000
RUN userdel -r ubuntu 2>/dev/null || true && \\
    groupadd -g 1000 yap && \\
    useradd -m -u 1000 -g 1000 -s /bin/bash yap && \\
    echo '${sudoers}' >> /etc/sudoers

APT
}

block_apt_debian() {
  local sudoers="$1"
  cat <<DEB
# Use bash with pipefail for safer pipes in RUN instructions (hadolint DL4006)
SHELL ["/bin/bash", "-o", "pipefail", "-c"]

# Environment variables
ENV DEBIAN_FRONTEND=noninteractive

# Install minimal runtime dependencies
RUN apt-get update && \\
    apt-get install -y --no-install-recommends \\
    bash-completion \\
    ca-certificates \\
    sudo && \\
    apt-get clean && \\
    rm -rf /var/lib/apt/lists/*

# Create non-root user
RUN groupadd -g 1000 yap && \\
    useradd -m -u 1000 -g 1000 -s /bin/bash yap && \\
    echo '${sudoers}' >> /etc/sudoers

DEB
}

block_dnf() {
  local sudoers="$1"
  cat <<DNF
# Install minimal runtime dependencies
RUN dnf -y update && \\
    dnf -y install \\
        bash-completion \\
        ca-certificates \\
        findutils \\
        sudo && \\
    dnf clean all && rm -rf /var/cache/dnf

# Create non-root user
RUN groupadd -g 1000 yap && \\
    useradd -m -u 1000 -g 1000 -s /bin/bash yap && \\
    echo '${sudoers}' >> /etc/sudoers

DNF
}

block_dnf_rocky() {
  local version_comment="$1" enable_cmd="$2" sudoers="$3"
  cat <<ROCKY
# Install minimal runtime dependencies. EPEL must be enabled first because
# ccache lives there on ${version_comment}; the package is not available in the base
# repos and dnf does not refresh metadata within a single transaction.
RUN dnf -y update && \\
    dnf -y install \\
        bash-completion \\
        ca-certificates \\
        dnf-plugins-core \\
        epel-release \\
        findutils \\
        sudo && \\
    ${enable_cmd} && \\
    dnf config-manager --enable devel && \\
    dnf -y install ccache && \\
    dnf clean all && rm -rf /var/cache/dnf

# The Rocky ccache package installs compiler symlinks under /usr/lib64/ccache;
# placing that directory first in PATH wraps gcc/g++ transparently.
ENV PATH="/usr/lib64/ccache:\${PATH}"

# Create non-root user
RUN groupadd -g 1000 yap && \\
    useradd -m -u 1000 -g 1000 -s /bin/bash yap && \\
    echo '${sudoers}' >> /etc/sudoers

ROCKY
}

block_yum() {
  local sudoers="$1"
  cat <<YUM
# Install minimal runtime dependencies
RUN yum -y update && yum -y install \\
    bash-completion \\
    ca-certificates \\
    findutils \\
    sudo && \\
    yum clean all && rm -rf /var/cache/yum

# Create non-root user
RUN groupadd -g 1000 yap && \\
    useradd -m -u 1000 -g 1000 -s /bin/bash yap && \\
    echo '${sudoers}' >> /etc/sudoers

YUM
}

block_zypper() {
  local sudoers="$1"
  cat <<ZYPPER
# Install minimal runtime dependencies
RUN zypper -n update && zypper -n install \\
    bash-completion \\
    ca-certificates \\
    sudo && \\
    zypper clean -a

# Create non-root user
RUN groupadd -g 1000 yap && \\
    useradd -m -u 1000 -g 1000 -s /bin/bash yap && \\
    echo '${sudoers}' >> /etc/sudoers

ZYPPER
}

block_pacman() {
  local sudoers="$1"
  cat <<PACMAN
# Install minimal runtime dependencies
RUN pacman -Syu --noconfirm && pacman -S --noconfirm \\
    bash-completion \\
    ca-certificates \\
    sudo && \\
    rm -rf /var/cache/pacman/pkg/*

# Create non-root user
RUN groupadd -g 1000 yap && \\
    useradd -m -u 1000 -g 1000 -s /bin/bash yap && \\
    echo '${sudoers}' >> /etc/sudoers

PACMAN
}

block_apk() {
  local sudoers="$1"
  cat <<APK
# Install minimal runtime dependencies
RUN apk add --no-cache \\
    bash \\
    bash-completion \\
    ca-certificates \\
    sudo

# Create non-root user
RUN addgroup -g 1000 yap && \\
    adduser -D -u 1000 -G yap -s /bin/bash yap && \\
    echo '${sudoers}' >> /etc/sudoers

APK
}

# ── Generator ─────────────────────────────────────────────────────────────────
# Write sections directly to avoid $() stripping trailing newlines.
generate() {
  local dir="$1"; shift
  local out="${REPO_ROOT}/build/deploy/${dir}/Dockerfile"
  mkdir -p "${REPO_ROOT}/build/deploy/${dir}"
  : > "$out"
  for fn in "$@"; do
    $fn >> "$out"
  done
  echo "  generated build/deploy/${dir}/Dockerfile"
}

APT_SUDO="yap ALL=(ALL) NOPASSWD: /usr/bin/yap, /usr/bin/tee, /usr/bin/apt-get, /usr/bin/dpkg, /usr/sbin/update-ccache-symlinks"
DEB_SUDO="yap ALL=(ALL) NOPASSWD: /usr/bin/yap, /usr/bin/tee, /usr/bin/apt-get"
RPM_SUDO="yap ALL=(ALL) NOPASSWD: /usr/bin/yap, /usr/bin/tee, /usr/bin/yum, /usr/bin/dnf"
ZYPPER_SUDO="yap ALL=(ALL) NOPASSWD: /usr/bin/yap, /usr/bin/tee, /usr/bin/zypper"
PACMAN_SUDO="yap ALL=(ALL) NOPASSWD: /usr/bin/yap, /usr/bin/tee, /usr/bin/pacman"
APK_SUDO="yap ALL=(ALL) NOPASSWD: /usr/bin/yap, /usr/bin/tee, /sbin/apk"

echo "Generating Dockerfiles..."

# Ubuntu (apt + ccache + userdel ubuntu)
for distro in ubuntu-focal ubuntu-jammy ubuntu-noble ubuntu-resolute; do
  case "$distro" in
    ubuntu-focal)    base="ubuntu:focal";    title="yap-ubuntu-focal";    desc="YAP - Yet Another Packager for Ubuntu 20.04 LTS with Go runtime 📦🐹" ;;
    ubuntu-jammy)    base="ubuntu:jammy";    title="yap-ubuntu-jammy";    desc="YAP - Yet Another Packager for Ubuntu 22.04 LTS with Go runtime 📦🐹" ;;
    ubuntu-noble)    base="ubuntu:noble";    title="yap-ubuntu-noble";    desc="YAP - Yet Another Packager for Ubuntu 24.04 LTS with Go runtime 📦🐹" ;;
    ubuntu-resolute) base="ubuntu:resolute"; title="yap-ubuntu-resolute"; desc="YAP - Yet Another Packager for Ubuntu 25.04 with Go runtime 📦🐹" ;;
  esac
  out="${REPO_ROOT}/build/deploy/${distro}/Dockerfile"
  mkdir -p "$(dirname "$out")"
  { builder_stage; runtime_header "$base" "$title" "$desc"; block_apt "$APT_SUDO"; runtime_footer; } > "$out"
  echo "  generated build/deploy/${distro}/Dockerfile"
done

# Debian (apt, no ccache, no userdel)
for distro in debian-buster debian-jessie debian-stretch; do
  case "$distro" in
    debian-buster)  base="debian:buster";  title="yap-debian-buster";  desc="YAP - Yet Another Packager for Debian Buster with Go runtime 📦🐹" ;;
    debian-jessie)  base="debian:jessie";  title="yap-debian-jessie";  desc="YAP - Yet Another Packager for Debian Jessie with Go runtime 📦🐹" ;;
    debian-stretch) base="debian:stretch"; title="yap-debian-stretch"; desc="YAP - Yet Another Packager for Debian Stretch with Go runtime 📦🐹" ;;
  esac
  out="${REPO_ROOT}/build/deploy/${distro}/Dockerfile"
  mkdir -p "$(dirname "$out")"
  { builder_stage; runtime_header "$base" "$title" "$desc"; block_apt_debian "$DEB_SUDO"; runtime_footer; } > "$out"
  echo "  generated build/deploy/${distro}/Dockerfile"
done

# Rocky (dnf + EPEL + ccache, powertools vs crb)
_gen_rocky() {
  local distro="$1" base="$2" title="$3" desc="$4" enable_cmd="$5"
  local out="${REPO_ROOT}/build/deploy/${distro}/Dockerfile"
  mkdir -p "$(dirname "$out")"
  { builder_stage; runtime_header "$base" "$title" "$desc"; block_dnf_rocky "${title#yap-}" "$enable_cmd" "$RPM_SUDO"; runtime_footer; } > "$out"
  echo "  generated build/deploy/${distro}/Dockerfile"
}
_gen_rocky "rocky-8"  "rockylinux/rockylinux:8"  "yap-rocky-8"  "YAP - Yet Another Packager for Rocky-8 with Go runtime 📦🐹"  "dnf config-manager --enable powertools"
_gen_rocky "rocky-9"  "rockylinux/rockylinux:9"  "yap-rocky-9"  "YAP - Yet Another Packager for Rocky-9 with Go runtime 📦🐹"  "dnf config-manager --enable crb"
_gen_rocky "rocky-10" "rockylinux/rockylinux:10" "yap-rocky-10" "YAP - Yet Another Packager for Rocky-10 with Go runtime 📦🐹" "dnf config-manager --enable crb"

# Helper for simple single-block distros
_gen() {
  local distro="$1" base="$2" title="$3" desc="$4" block_fn="$5"; shift 5
  local out="${REPO_ROOT}/build/deploy/${distro}/Dockerfile"
  mkdir -p "$(dirname "$out")"
  { builder_stage; runtime_header "$base" "$title" "$desc"; "$block_fn" "$@"; runtime_footer; } > "$out"
  echo "  generated build/deploy/${distro}/Dockerfile"
}

# Fedora (plain dnf)
_gen "fedora-38" "fedora:38" "yap-fedora-38" "YAP - Yet Another Packager for Fedora-38 with Go runtime 📦🐹" block_dnf "$RPM_SUDO"

# Amazon Linux (yum)
_gen "amazon-1" "amazonlinux:1" "yap-amazon-1" "YAP - Yet Another Packager for Amazon-1 with Go runtime 📦🐹" block_yum "$RPM_SUDO"
_gen "amazon-2" "amazonlinux:2" "yap-amazon-2" "YAP - Yet Another Packager for Amazon-2 with Go runtime 📦🐹" block_yum "$RPM_SUDO"

# openSUSE (zypper)
_gen "opensuse-leap"      "opensuse/leap:latest"       "yap-opensuse-leap"       "YAP - Yet Another Packager for openSUSE Leap with Go runtime 📦🐹"       block_zypper "$ZYPPER_SUDO"
_gen "opensuse-tubleweed" "opensuse/tumbleweed:latest" "yap-opensuse-tumbleweed" "YAP - Yet Another Packager for openSUSE Tumbleweed with Go runtime 📦🐹" block_zypper "$ZYPPER_SUDO"

# Arch (pacman)
_gen "arch" "archlinux:latest" "yap-arch" "YAP - Yet Another Packager for Arch Linux with Go runtime 📦🐹" block_pacman "$PACMAN_SUDO"

# Alpine (apk)
_gen "alpine" "alpine:latest" "yap-alpine" "YAP - Yet Another Packager for Alpine Linux with Go runtime 📦🐹" block_apk "$APK_SUDO"

echo "Done. Run 'git diff build/deploy/' to review changes."
