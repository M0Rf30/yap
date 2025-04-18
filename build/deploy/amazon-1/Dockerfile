FROM docker.io/amazonlinux:1 AS builder
LABEL org.opencontainers.image.title="yap"
LABEL org.opencontainers.image.description="Package software with ease 📦"
LABEL org.opencontainers.image.vendor="M0Rf30"

SHELL ["/bin/bash", "-o", "pipefail", "-c"]
RUN \
    yum -y install curl tar; \
    curl -sL https://go.dev/dl/go1.24.2.linux-amd64.tar.gz --output go.tar.gz; \
    echo "68097bd680839cbc9d464a0edce4f7c333975e27a90246890e9f1078c7e702ad go.tar.gz" | sha256sum -c - ; \
    tar -C /usr/lib -xf go.tar.gz; \
    ln -sf /usr/lib/go/bin/go /usr/bin/go; \
    ln -sf /usr/lib/go/bin/gofmt /usr/bin/gofmt; \
    ln -sf /usr/share/doc/go /usr/lib/go/doc; \
    yum clean all

COPY . /project
WORKDIR /project
RUN \
    CGO_ENABLED=0 \
    go build \
    -buildvcs=false \
    -ldflags "-w -s" \
    -o /usr/bin/yap \
    cmd/yap/yap.go

FROM docker.io/amazonlinux:1
COPY --from=builder /usr/bin/yap /usr/bin/yap
RUN \
    yum -y install \
    bash-completion \
    ca-certificates \
    findutils \
    sudo; \
    yap completion bash > /etc/bash_completion.d/yap; \
    echo "source /usr/share/bash-completion/bash_completion" >> /etc/bashrc; \
    yum clean all

ENV GOROOT=/usr/lib/go
ENV PATH /usr/lib/go/bin:$PATH:/go/bin

ENTRYPOINT ["yap"]
