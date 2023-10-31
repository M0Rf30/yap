FROM docker.io/opensuse/tumbleweed:latest AS builder
LABEL org.opencontainers.image.title="yap"
LABEL org.opencontainers.image.description="Package software with ease 📦"
LABEL org.opencontainers.image.vendor="M0Rf30"

SHELL ["/bin/bash", "-o", "pipefail", "-c"]
RUN \
  zypper -n install -y go

COPY . /project
WORKDIR /project
RUN \
  go build \
    -buildvcs=false \
    -ldflags "-w -s" \
    -mod=readonly \
    -modcacherw \
    -trimpath \
    -o /usr/bin/yap \
    cmd/yap/yap.go

FROM docker.io/opensuse/tumbleweed:latest
COPY --from=builder /usr/bin/yap /usr/bin/yap
RUN \
  zypper -n install -y \
    bash-completion \
    ca-certificates \
    sudo; \
  yap completion bash > /usr/share/bash-completion/completions/yap; \
  echo "source /usr/share/bash-completion/bash_completion" >> /etc/bashrc;

ENTRYPOINT ["yap"]
