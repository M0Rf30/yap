FROM debian:jessie
LABEL org.opencontainers.image.title="yap"
LABEL org.opencontainers.image.description="Package software with ease 📦"
LABEL org.opencontainers.image.vendor="M0Rf30"

ENV DEBIAN_FRONTEND=noninteractive
SHELL ["/bin/bash", "-o", "pipefail", "-c"]
RUN \
  apt-get update; \
  apt-get upgrade; \
  apt-get install -y --no-install-recommends \
  bash-completion build-essential reprepro rsync zip git sudo curl tzdata ca-certificates; \
  curl -sL https://dl.google.com/go/go1.19.2.linux-amd64.tar.gz --output go.tar.gz; \
  echo "5e8c5a74fe6470dd7e055a461acda8bb4050ead8c2df70f227e3ff7d8eb7eeb6 go.tar.gz" | sha256sum -c - ; \
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
