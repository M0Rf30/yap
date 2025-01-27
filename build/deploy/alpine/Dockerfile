FROM docker.io/alpine:latest AS builder
LABEL org.opencontainers.image.title="yap"
LABEL org.opencontainers.image.description="Package software with ease 📦"
LABEL org.opencontainers.image.vendor="M0Rf30"

RUN \
  apk add --no-cache upx; \
  wget https://go.dev/dl/go1.23.5.linux-amd64.tar.gz -O go.tar.gz; \
  echo "cbcad4a6482107c7c7926df1608106c189417163428200ce357695cc7e01d091 go.tar.gz" | sha256sum -c - ; \
  tar -C /usr/lib -xf go.tar.gz; \
  ln -sf /usr/lib/go/bin/go /usr/bin/go; \
  ln -sf /usr/lib/go/bin/gofmt /usr/bin/gofmt; \
  ln -sf /usr/share/doc/go /usr/lib/go/doc;

COPY . /project
WORKDIR /project
RUN \
  CGO_ENABLED=0 \
    go build \
    -buildvcs=false \
    -ldflags "-w -s" \
    -o /usr/bin/yap \
    cmd/yap/yap.go; \
    upx /usr/bin/yap

FROM docker.io/alpine:latest
COPY --from=builder /usr/bin/yap /usr/bin/yap
RUN \
  apk add --no-cache \
    bash-completion \
    ca-certificates \
    sudo; \
  yap completion bash > /etc/bash_completion.d/yap; \
  echo "source /usr/share/bash-completion/bash_completion" >> /etc/bashrc;

ENTRYPOINT ["yap"]
