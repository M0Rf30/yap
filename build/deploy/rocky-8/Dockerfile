FROM docker.io/rockylinux/rockylinux:8 AS builder
LABEL org.opencontainers.image.title="yap"
LABEL org.opencontainers.image.description="Package software with ease 📦"
LABEL org.opencontainers.image.vendor="M0Rf30"

SHELL ["/bin/bash", "-o", "pipefail", "-c"]
RUN \
    dnf -y install curl tar; \
    curl -sL https://go.dev/dl/go1.26.2.linux-amd64.tar.gz --output go.tar.gz; \
    echo "990e6b4bbba816dc3ee129eaeaf4b42f17c2800b88a2166c265ac1a200262282 go.tar.gz" | sha256sum -c - ; \
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

FROM docker.io/rockylinux/rockylinux:8
COPY --from=builder /usr/bin/yap /usr/bin/yap
RUN \
    dnf -y install \
    bash-completion \
    binutils \
    ca-certificates \
    dnf-plugins-core \
    epel-release \
    findutils \
    sudo; \
    dnf config-manager --enable devel; \
    dnf config-manager --enable powertools; \
    yap completion bash > /etc/bash_completion.d/yap; \
    echo "source /usr/share/bash-completion/bash_completion" >> /etc/bashrc; \
    dnf clean all

ENV GOROOT=/usr/lib/go
ENV PATH=/usr/lib/go/bin:$PATH:/go/bin

ENTRYPOINT ["yap"]
