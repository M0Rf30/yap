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
  curl -sL https://go.dev/dl/go1.20.5.linux-amd64.tar.gz --output go.tar.gz; \
  echo "d7ec48cde0d3d2be2c69203bc3e0a44de8660b9c09a6e85c4732a3f7dc442612 go.tar.gz" | sha256sum -c - ; \
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
  apt-get install -y --no-install-recommends bash-completion sudo; \
  yap completion bash > /usr/share/bash-completion/completions/yap; \
  echo "source /usr/share/bash-completion/bash_completion" >> /etc/bash.bashrc; \
  apt-get clean; \
  rm -rf /var/lib/apt/lists/*

ENV GOROOT=/usr/lib/go
ENV PATH /usr/lib/go/bin:$PATH:/go/bin

ENTRYPOINT ["yap"]
