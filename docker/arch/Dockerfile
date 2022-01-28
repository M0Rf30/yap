FROM archlinux:base
LABEL maintainer="Yap <contact@packagefoundation.org>"

SHELL ["/bin/bash", "-o", "pipefail", "-c"]
RUN \
  pacman -Syu --noconfirm; \
  pacman -S --noconfirm base-devel bash-completion git go rsync sudo tar zip; \
  rm -rf /var/cache/pacman/*

COPY . /project
WORKDIR /project
RUN \
  go build -ldflags "-w -s" -o /usr/bin/; \
  yap completion bash > /usr/share/bash-completion/completions/yap; \
  echo "source /usr/share/bash-completion/bash_completion" >> /etc/bashrc; \
  rm -rf /project/*

ENTRYPOINT ["yap"]