# syntax=docker/dockerfile:1
# Minimal OCI image shipping only the yap-mcp stdio server.
# Published as ghcr.io/m0rf30/yap-mcp and referenced by server.json for the
# Model Context Protocol registry (registryType: oci).

FROM --platform=$BUILDPLATFORM golang:1.26-alpine AS build
WORKDIR /src
RUN apk add --no-cache git
COPY go.mod go.sum ./
RUN go mod download
COPY . .
# Cross-compile natively on the build platform (no QEMU). CGO is disabled, so
# GOARCH switching is clean and needs no C toolchain.
ARG TARGETOS
ARG TARGETARCH
ARG VERSION=dev
ARG COMMIT=none
ARG BUILD_TIME=unknown
RUN CGO_ENABLED=0 GOOS=${TARGETOS} GOARCH=${TARGETARCH} go build \
    -ldflags "-s -w \
      -X github.com/M0Rf30/yap/v2/pkg/buildinfo.Version=${VERSION} \
      -X github.com/M0Rf30/yap/v2/pkg/buildinfo.Commit=${COMMIT} \
      -X github.com/M0Rf30/yap/v2/pkg/buildinfo.BuildTime=${BUILD_TIME}" \
    -o /out/yap-mcp ./cmd/yap-mcp

FROM gcr.io/distroless/static-debian12:nonroot
LABEL org.opencontainers.image.title="yap-mcp" \
      org.opencontainers.image.description="MCP server exposing YAP package-build capabilities" \
      org.opencontainers.image.source="https://github.com/M0Rf30/yap" \
      org.opencontainers.image.vendor="M0Rf30" \
      io.modelcontextprotocol.server.name="io.github.m0rf30/yap"
COPY --from=build /out/yap-mcp /usr/local/bin/yap-mcp
ENTRYPOINT ["/usr/local/bin/yap-mcp"]
