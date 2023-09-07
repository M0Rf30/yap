FROM docker.io/fedora:38 AS builder
LABEL org.opencontainers.image.title="yap"
LABEL org.opencontainers.image.description="Package software with ease 📦"
LABEL org.opencontainers.image.vendor="M0Rf30"

SHELL ["/bin/bash", "-o", "pipefail", "-c"]
RUN \
  dnf -y install curl tar; \
  curl -sL https://go.dev/dl/go1.21.1.linux-amd64.tar.gz --output go.tar.gz; \
  echo "b3075ae1ce5dab85f89bc7905d1632de23ca196bd8336afd93fa97434cfa55ae go.tar.gz" | sha256sum -c - ; \
  tar -C /usr/lib -xf go.tar.gz; \
  ln -sf /usr/lib/go/bin/go /usr/bin/go; \
  ln -sf /usr/lib/go/bin/gofmt /usr/bin/gofmt; \
  ln -sf /usr/share/doc/go /usr/lib/go/doc; \
  dnf clean all

COPY . /project
WORKDIR /project
RUN \
  go build -ldflags "-w -s" -trimpath -mod=readonly -modcacherw -o /usr/bin/;

FROM docker.io/fedora:38
COPY --from=builder /usr/bin/yap /usr/bin/yap
RUN \
  dnf -y install bash-completion sudo; \
  yap completion bash > /etc/bash_completion.d/yap; \
  echo "source /usr/share/bash-completion/bash_completion" >> /etc/bashrc; \
  dnf clean all

ENV GOROOT=/usr/lib/go
ENV PATH /usr/lib/go/bin:$PATH:/go/bin

ENTRYPOINT ["yap"]
