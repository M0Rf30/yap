# syntax=docker/dockerfile:1
ARG GO_VERSION=1.26.4

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

# Runtime stage
FROM ubuntu:focal

# Build arguments for runtime stage
ARG VERSION=dev
ARG TARGETARCH=amd64
ARG GO_VERSION=1.26.4

# Metadata labels
LABEL org.opencontainers.image.title="yap-ubuntu-focal"
LABEL org.opencontainers.image.description="YAP - Yet Another Packager for Ubuntu 20.04 LTS with Go runtime 📦🐹"
LABEL org.opencontainers.image.vendor="M0Rf30"
LABEL org.opencontainers.image.source="https://github.com/M0Rf30/yap"
LABEL org.opencontainers.image.licenses="GPL-3.0"
LABEL org.opencontainers.image.version="${VERSION}"

# Use bash with pipefail for safer pipes in RUN instructions (hadolint DL4006)
SHELL ["/bin/bash", "-o", "pipefail", "-c"]

# Environment variables
ENV DEBIAN_FRONTEND=noninteractive
ENV YAP_IN_CONTAINER=1

# Preseed debconf to prevent resolvconf postinst failure in containers
RUN echo "resolvconf resolvconf/linkify-resolvconf boolean false" | debconf-set-selections

# Install minimal runtime dependencies
RUN apt-get update && \
    apt-get upgrade -y && \
    apt-get install -y --no-install-recommends \
    bash-completion \
    binutils \
    ca-certificates \
    ccache \
    sudo && \
    # Set timezone
    ln -sf /usr/share/zoneinfo/UTC /etc/localtime && \
    # Clean up
    apt-get autoremove -y && \
    apt-get clean && \
    rm -rf /var/lib/apt/lists/*

# Make ccache visible to every compiler invocation. The Debian/Ubuntu ccache
# package installs compiler symlinks under /usr/lib/ccache; placing that
# directory first in PATH wraps gcc/g++ and cross-compilers transparently.
ENV PATH="/usr/lib/ccache:${PATH}"

# Remove default ubuntu user and create yap user at uid/gid 1000
RUN userdel -r ubuntu 2>/dev/null || true && \
    groupadd -g 1000 yap && \
    useradd -m -u 1000 -g 1000 -s /bin/bash yap && \
    echo 'Defaults env_keep += "KUBERNETES_SERVICE_HOST YAP_IN_CONTAINER YAP_ALLOW_UNVERIFIED_REPOS"' >> /etc/sudoers && echo 'yap ALL=(ALL) NOPASSWD: /usr/bin/yap, /usr/bin/tee, /usr/bin/apt-get, /usr/bin/dpkg, /usr/sbin/update-ccache-symlinks' >> /etc/sudoers

# Copy binary and completion from builder
COPY --from=builder /usr/bin/yap /usr/bin/yap
COPY --from=builder /tmp/yap-completion.bash /usr/share/bash-completion/completions/yap

# Set up bash completion
RUN echo "source /usr/share/bash-completion/bash_completion" >> /etc/bash.bashrc

# Switch to non-root user
USER yap

ENTRYPOINT ["yap"]
