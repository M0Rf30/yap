FROM ubuntu:bionic
LABEL maintainer="Yap <contact@packagefoundation.org>"

ENV DEBIAN_FRONTEND=noninteractive
SHELL ["/bin/bash", "-o", "pipefail", "-c"]
RUN \
  apt-get update; \
  apt-get upgrade; \
  apt-get install -y --no-install-recommends \
  bash-completion build-essential ca-certificates curl git reprepro rsync sudo tzdata zip; \
  curl -sL https://dl.google.com/go/go1.17.6.linux-amd64.tar.gz --output go.tar.gz; \
  echo "231654bbf2dab3d86c1619ce799e77b03d96f9b50770297c8f4dff8836fc8ca2 go.tar.gz" | sha256sum -c - ; \
  tar -C /usr/lib -xf go.tar.gz; \
  rm -f go.tar.gz; \
  ln -sf /usr/lib/go/bin/go /usr/bin/go; \
  ln -sf /usr/lib/go/bin/gofmt /usr/bin/gofmt; \
  ln -sf /usr/share/doc/go /usr/lib/go/doc; \
  apt-get clean; \
  rm -rf /var/lib/apt/lists/*

ENV GOROOT=/usr/lib/go
ENV PATH /usr/lib/go/bin:$PATH:/go/bin

COPY . /project
WORKDIR /project
RUN \
  go build -ldflags "-w -s" -trimpath -mod=readonly -modcacherw -o /usr/bin/; \
   yap completion bash > /etc/bash_completion.d/yap; \
   echo "source /usr/share/bash-completion/bash_completion" >> /etc/bash.bashrc; \
   rm -rf /project

ENTRYPOINT ["yap"]
