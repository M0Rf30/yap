FROM docker.io/ubuntu:focal AS builder
LABEL org.opencontainers.image.title="yap"
LABEL org.opencontainers.image.description="Package software with ease 📦"
LABEL org.opencontainers.image.vendor="M0Rf30"

ENV DEBIAN_FRONTEND=noninteractive
SHELL ["/bin/bash", "-o", "pipefail", "-c"]
RUN \
  apt-get update; \
  apt-get upgrade; \
  apt-get install -y --no-install-recommends ca-certificates curl tar; \
  curl -sL https://go.dev/dl/go1.21.6.linux-amd64.tar.gz --output go.tar.gz; \
  echo "3f934f40ac360b9c01f616a9aa1796d227d8b0328bf64cb045c7b8c4ee9caea4 go.tar.gz" | sha256sum -c - ; \
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
    cmd/yap/yap.go

FROM docker.io/ubuntu:focal
COPY --from=builder /usr/bin/yap /usr/bin/yap
RUN \
  apt-get update; \
  apt-get upgrade; \
  apt-get install -y --no-install-recommends \
    bash-completion \
    ca-certificates \
    sudo; \
  yap completion bash > /usr/share/bash-completion/completions/yap; \
  echo "source /usr/share/bash-completion/bash_completion" >> /etc/bash.bashrc; \
  apt-get clean; \
  rm -rf /var/lib/apt/lists/*

ENV DEBIAN_FRONTEND=noninteractive
ENV GOROOT=/usr/lib/go
ENV PATH /usr/lib/go/bin:$PATH:/go/bin

ENTRYPOINT ["yap"]
