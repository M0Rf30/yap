# syntax=docker/dockerfile:1
# Prepared variant: extends yap-ubuntu-noble with Go toolchain pre-installed via yap prepare -g.
ARG REGISTRY=ghcr.io/m0rf30/yap
ARG BASE_TAG=latest

FROM ${REGISTRY}-ubuntu-noble:${BASE_TAG}

# Build arguments for metadata
ARG VERSION=dev

# Metadata labels
LABEL org.opencontainers.image.title="yap-ubuntu-noble-g"
LABEL org.opencontainers.image.description="YAP - Yet Another Packager for ubuntu-noble with Go toolchain pre-installed 📦🐹"
LABEL org.opencontainers.image.vendor="M0Rf30"
LABEL org.opencontainers.image.source="https://github.com/M0Rf30/yap"
LABEL org.opencontainers.image.licenses="GPL-3.0"
LABEL org.opencontainers.image.version="${VERSION}"

# Switch to root (uid 0) to install Go toolchain
USER 0

# Pre-install the build toolchain and Go via yap prepare -g. The base image
# ships no package index (apt/apk caches are wiped to keep it small), so the
# metadata sync must run here or every build dependency is unresolvable.
RUN yap prepare ubuntu-noble --no-container -g

# Switch back to non-root user (uid 1000 = yap)
USER 1000

ENTRYPOINT ["yap"]
