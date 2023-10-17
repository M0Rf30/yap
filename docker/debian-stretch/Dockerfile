FROM docker.io/debian:stretch AS builder
LABEL org.opencontainers.image.title="yap"
LABEL org.opencontainers.image.description="Package software with ease 📦"
LABEL org.opencontainers.image.vendor="M0Rf30"

ENV DEBIAN_FRONTEND=noninteractive
SHELL ["/bin/bash", "-o", "pipefail", "-c"]
RUN \
  apt-get update; \
  apt-get upgrade; \
  apt-get install -y --no-install-recommends ca-certificates curl tar; \
  curl -sL https://go.dev/dl/go1.21.3.linux-amd64.tar.gz --output go.tar.gz; \
  echo "1241381b2843fae5a9707eec1f8fb2ef94d827990582c7c7c32f5bdfbfd420c8 go.tar.gz" | sha256sum -c - ; \
  tar -C /usr/lib -xf go.tar.gz; \
  ln -sf /usr/lib/go/bin/go /usr/bin/go; \
  ln -sf /usr/lib/go/bin/gofmt /usr/bin/gofmt; \
  ln -sf /usr/share/doc/go /usr/lib/go/doc;

COPY . /project
WORKDIR /project
RUN \
  go build -ldflags "-w -s" -trimpath -mod=readonly -modcacherw -o /usr/bin/;

FROM docker.io/debian:stretch
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

ENV GOROOT=/usr/lib/go
ENV PATH /usr/lib/go/bin:$PATH:/go/bin

ENTRYPOINT ["yap"]
