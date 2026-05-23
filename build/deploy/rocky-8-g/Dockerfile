# syntax=docker/dockerfile:1
# Prepared variant: extends yap-rocky-8 with Go toolchain pre-installed via yap prepare -g.
ARG REGISTRY=ghcr.io/m0rf30/yap
ARG BASE_TAG=latest

FROM ${REGISTRY}-rocky-8:${BASE_TAG}

# Build arguments for metadata
ARG VERSION=dev

# Metadata labels
LABEL org.opencontainers.image.title="yap-rocky-8-g"
LABEL org.opencontainers.image.description="YAP - Yet Another Packager for rocky-8 with Go toolchain pre-installed 📦🐹"
LABEL org.opencontainers.image.vendor="M0Rf30"
LABEL org.opencontainers.image.source="https://github.com/M0Rf30/yap"
LABEL org.opencontainers.image.licenses="GPL-3.0"
LABEL org.opencontainers.image.version="${VERSION}"

# Switch to root to install Go toolchain
USER root

# Pre-install Go toolchain via yap prepare -g
RUN yap prepare rocky-8 --no-container --skip-sync -g

# Switch back to non-root user
USER yap

ENTRYPOINT ["yap"]
