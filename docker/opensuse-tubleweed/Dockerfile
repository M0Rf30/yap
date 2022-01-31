FROM opensuse/tumbleweed:latest
LABEL maintainer="Yap <contact@packagefoundation.org>"

RUN \
  zypper -n install -y automake bash-completion expect gcc git make rpm-build rsync tar unzip zip; \
  zypper -n install -y go

COPY . /project
WORKDIR /project
RUN \
  go build -ldflags "-w -s" -trimpath -mod=readonly -modcacherw -o /usr/bin/; \
  yap completion bash > /usr/share/bash-completion/completions/yap; \
  echo "source /usr/share/bash-completion/bash_completion" >> /etc/bashrc; \
  rm -rf /project

ENTRYPOINT ["yap"]
