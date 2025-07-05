FROM docker.io/ubuntu:jammy AS builder
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
    curl -sL https://go.dev/dl/go1.24.3.linux-amd64.tar.gz --output go.tar.gz; \
    echo "3333f6ea53afa971e9078895eaa4ac7204a8c6b5c68c10e6bc9a33e8e391bdd8 go.tar.gz" | sha256sum -c - ; \
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

FROM docker.io/ubuntu:jammy
COPY --from=builder /usr/bin/yap /usr/bin/yap
RUN \
    apt-get update; \
    apt-get upgrade; \
    apt-get install -y --no-install-recommends \
    bash-completion \
    binutils \
    ca-certificates \
    sudo; \
    ln -sf /usr/share/zoneinfo/UTC /etc/localtime; \
    yap completion bash > /usr/share/bash-completion/completions/yap; \
    echo "source /usr/share/bash-completion/bash_completion" >> /etc/bash.bashrc; \
    apt-get clean; \
    rm -rf /var/lib/apt/lists/*

ENV DEBIAN_FRONTEND=noninteractive
ENV GOROOT=/usr/lib/go
ENV PATH=/usr/lib/go/bin:$PATH:/go/bin

ENTRYPOINT ["yap"]
