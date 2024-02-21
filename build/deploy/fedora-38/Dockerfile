FROM docker.io/fedora:38 AS builder
LABEL org.opencontainers.image.title="yap"
LABEL org.opencontainers.image.description="Package software with ease 📦"
LABEL org.opencontainers.image.vendor="M0Rf30"

SHELL ["/bin/bash", "-o", "pipefail", "-c"]
RUN \
  dnf -y install curl tar; \
  curl -sL https://go.dev/dl/go1.22.0.linux-amd64.tar.gz --output go.tar.gz; \
  echo "f6c8a87aa03b92c4b0bf3d558e28ea03006eb29db78917daec5cfb6ec1046265 go.tar.gz" | sha256sum -c - ; \
  tar -C /usr/lib -xf go.tar.gz; \
  ln -sf /usr/lib/go/bin/go /usr/bin/go; \
  ln -sf /usr/lib/go/bin/gofmt /usr/bin/gofmt; \
  ln -sf /usr/share/doc/go /usr/lib/go/doc; \
  dnf clean all

COPY . /project
WORKDIR /project
RUN \
  CGO_ENABLED=0 \
    go build \
    -buildvcs=false \
    -ldflags "-w -s" \
    -o /usr/bin/yap \
    cmd/yap/yap.go

FROM docker.io/fedora:38
COPY --from=builder /usr/bin/yap /usr/bin/yap
RUN \
  dnf -y install \
    bash-completion \
    ca-certificates \
    rpm-build \
    sudo; \
  yap completion bash > /etc/bash_completion.d/yap; \
  echo "source /usr/share/bash-completion/bash_completion" >> /etc/bashrc; \
  dnf clean all

ENV GOROOT=/usr/lib/go
ENV PATH /usr/lib/go/bin:$PATH:/go/bin

ENTRYPOINT ["yap"]
