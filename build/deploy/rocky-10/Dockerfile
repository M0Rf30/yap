# syntax=docker/dockerfile:1
ARG GO_VERSION=1.24.5

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
FROM rockylinux:10

# Build arguments for runtime stage
ARG VERSION=dev
ARG TARGETARCH=amd64
ARG GO_VERSION=1.24.5

# Metadata labels
LABEL org.opencontainers.image.title="yap-rocky-10"
LABEL org.opencontainers.image.description="YAP - Yet Another Packager for Rocky-10 with Go runtime 📦🐹"
LABEL org.opencontainers.image.vendor="M0Rf30"
LABEL org.opencontainers.image.source="https://github.com/M0Rf30/yap"
LABEL org.opencontainers.image.licenses="GPL-3.0"
LABEL org.opencontainers.image.version="${VERSION}"


# Go environment variables
ENV GOROOT=/usr/local/go
ENV GOPATH=/go
ENV PATH=$GOROOT/bin:$GOPATH/bin:$PATH

# Install minimal runtime dependencies and Go
RUN dnf -y update && dnf -y install \
        bash-completion \
        ca-certificates \
        sudo \
        wget && \
    # Download and install Go (official binaries, no checksums)
    case "${TARGETARCH}" in \
        amd64) GO_ARCH=amd64 ;; \
        arm64) GO_ARCH=arm64 ;; \
        *) GO_ARCH=amd64 ;; \
    esac && \
    wget --progress=dot:giga -O go.tar.gz "https://go.dev/dl/go${GO_VERSION}.linux-${GO_ARCH}.tar.gz" && \
    tar -C /usr/local -xzf go.tar.gz && \
    rm go.tar.gz && \
    # Remove wget to minimize final image
    dnf remove -y wget && \
    dnf clean all && rm -rf /var/cache/dnf

# Create non-root user
RUN groupadd -g 1000 yap && \
    useradd -m -u 1000 -g yap -s /bin/bash yap && \
    echo 'yap ALL=(ALL) NOPASSWD: /usr/bin/yap' >> /etc/sudoers

# Copy binary and completion from builder
COPY --from=builder /usr/bin/yap /usr/bin/yap
COPY --from=builder /tmp/yap-completion.bash /usr/share/bash-completion/completions/yap

# Set up bash completion
RUN echo "source /usr/share/bash-completion/bash_completion" >> /etc/bash.bashrc

# Switch to non-root user
USER yap

ENTRYPOINT ["yap"]
