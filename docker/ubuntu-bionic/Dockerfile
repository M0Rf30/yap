FROM ubuntu:bionic
LABEL org.opencontainers.image.title="yap"
LABEL org.opencontainers.image.description="Package software with ease 📦"
LABEL org.opencontainers.image.vendor="M0Rf30"

ENV DEBIAN_FRONTEND=noninteractive
SHELL ["/bin/bash", "-o", "pipefail", "-c"]
RUN \
  apt-get update; \
  apt-get upgrade; \
  apt-get install -y --no-install-recommends \
  bash-completion build-essential ca-certificates curl git reprepro rsync sudo tzdata zip; \
  curl -sL https://dl.google.com/go/go1.20.linux-amd64.tar.gz --output go.tar.gz; \
  echo "5a9ebcc65c1cce56e0d2dc616aff4c4cedcfbda8cc6f0288cc08cda3b18dcbf1 go.tar.gz" | sha256sum -c - ; \
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
