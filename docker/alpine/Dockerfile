FROM alpine:latest
LABEL maintainer="Yap <contact@packagefoundation.org>"

RUN \
  apk add --no-cache  \
  alpine-sdk bash bash-completion git go rsync sudo tar zip;

COPY . /project
WORKDIR /project
RUN \
  go build -buildvcs=false -ldflags "-w -s" -trimpath -mod=readonly -modcacherw -o /usr/bin/; \
  yap completion bash > /usr/share/bash-completion/completions/yap; \
  echo "source /usr/share/bash-completion/bash_completion" >> /etc/bashrc; \
  rm -rf /project

ENTRYPOINT ["yap"]
