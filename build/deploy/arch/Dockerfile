# syntax=docker/dockerfile:1
ARG GO_VERSION=1.25.4

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
    -ldflags "-w -s -X main.version=${VERSION} -X main.commit=${COMMIT} -X main.buildTime=${BUILD_TIME}" \
    -o /usr/bin/yap \
    cmd/yap/yap.go && \
    upx --best --lzma /usr/bin/yap

# Generate bash completion (do this in build stage where we can execute the binary)
RUN if [ "${TARGETARCH}" = "$(go env GOARCH)" ]; then \
    /usr/bin/yap completion bash > /tmp/yap-completion.bash; \
    else \
    echo "# Cross-compilation: bash completion will be generated at runtime" > /tmp/yap-completion.bash; \
    fi

# Runtime stage
FROM archlinux:latest

# Build arguments for runtime stage
ARG VERSION=dev
ARG TARGETARCH=amd64
ARG GO_VERSION=1.25.4

# Metadata labels
LABEL org.opencontainers.image.title="yap-arch"
LABEL org.opencontainers.image.description="YAP - Yet Another Packager for Arch with Go runtime 📦🐹"
LABEL org.opencontainers.image.vendor="M0Rf30"
LABEL org.opencontainers.image.source="https://github.com/M0Rf30/yap"
LABEL org.opencontainers.image.licenses="GPL-3.0"
LABEL org.opencontainers.image.version="${VERSION}"


# Install minimal runtime dependencies
RUN pacman -Syu --noconfirm && pacman -S --noconfirm \
    bash-completion \
    ca-certificates \
    sudo && \
    pacman -Scc --noconfirm && rm -rf /var/cache/pacman/pkg/*

# Create non-root user
RUN groupadd -g 1001 yap && \
    useradd -m -u 1001 -g 1001 -s /bin/bash yap && \
    echo 'yap ALL=(ALL) NOPASSWD: /usr/bin/yap' >> /etc/sudoers

# Copy binary and completion from builder
COPY --from=builder /usr/bin/yap /usr/bin/yap
COPY --from=builder /tmp/yap-completion.bash /usr/share/bash-completion/completions/yap

# Set up bash completion
RUN echo "source /usr/share/bash-completion/bash_completion" >> /etc/bash.bashrc

# Switch to non-root user
USER yap

ENTRYPOINT ["yap"]
