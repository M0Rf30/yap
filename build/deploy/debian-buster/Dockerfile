FROM docker.io/debian:buster AS builder
LABEL org.opencontainers.image.title="yap"
LABEL org.opencontainers.image.description="Package software with ease 📦"
LABEL org.opencontainers.image.vendor="M0Rf30"

ENV DEBIAN_FRONTEND=noninteractive
SHELL ["/bin/bash", "-o", "pipefail", "-c"]
RUN \
    apt-get update; \
    apt-get upgrade; \
    apt-get install -y --no-install-recommends \
    ca-certificates \
    curl \
    tar \
    upx-ucl; \
    curl -sL https://go.dev/dl/go1.24.0.linux-amd64.tar.gz --output go.tar.gz; \
    echo "dea9ca38a0b852a74e81c26134671af7c0fbe65d81b0dc1c5bfe22cf7d4c8858 go.tar.gz" | sha256sum -c - ; \
    tar -C /usr/lib -xf go.tar.gz; \
    ln -sf /usr/lib/go/bin/go /usr/bin/go; \
    ln -sf /usr/lib/go/bin/gofmt /usr/bin/gofmt; \
    ln -sf /usr/share/doc/go /usr/lib/go/doc;

COPY . /project
WORKDIR /project
RUN \
    CGO_ENABLED=0 \
    go build \
    -buildvcs=false \
    -ldflags "-w -s" \
    -o /usr/bin/yap \
    cmd/yap/yap.go; \
    upx /usr/bin/yap

FROM docker.io/debian:buster
COPY --from=builder /usr/bin/yap /usr/bin/yap
RUN \
    apt-get update; \
    apt-get upgrade; \
    apt-get install -y --no-install-recommends \
    bash-completion \
    ca-certificates \
    sudo; \
    ln -sf /usr/share/zoneinfo/UTC /etc/localtime; \
    yap completion bash > /usr/share/bash-completion/completions/yap; \
    echo "source /usr/share/bash-completion/bash_completion" >> /etc/bash.bashrc; \
    apt-get clean; \
    rm -rf /var/lib/apt/lists/*

ENV DEBIAN_FRONTEND=noninteractive
ENV GOROOT=/usr/lib/go
ENV PATH /usr/lib/go/bin:$PATH:/go/bin

ENTRYPOINT ["yap"]
