FROM docker.io/archlinux:base AS builder
LABEL org.opencontainers.image.title="yap"
LABEL org.opencontainers.image.description="Package software with ease 📦"
LABEL org.opencontainers.image.vendor="M0Rf30"

SHELL ["/bin/bash", "-o", "pipefail", "-c"]
RUN \
  pacman -Syu --noconfirm go

COPY . /project
WORKDIR /project
RUN \
  go build -ldflags "-w -s" -trimpath -mod=readonly -modcacherw -o /usr/bin/

FROM docker.io/archlinux:base
COPY --from=builder /usr/bin/yap /usr/bin/yap
RUN \
  pacman -Syu --noconfirm \
    bash-completion \
    ca-certificates \
    sudo; \
  rm -rf /var/cache/pacman/* \
  yap completion bash > /etc/bash_completion.d/yap; \
  echo "source /usr/share/bash-completion/bash_completion" >> /etc/bashrc;

ENTRYPOINT ["yap"]
