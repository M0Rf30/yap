FROM docker.io/alpine:latest AS builder
LABEL org.opencontainers.image.title="yap"
LABEL org.opencontainers.image.description="Package software with ease 📦"
LABEL org.opencontainers.image.vendor="M0Rf30"

RUN \
  apk add --no-cache go

COPY . /project
WORKDIR /project
RUN \
  go build \
    -buildvcs=false \
    -ldflags "-w -s" \
    -mod=readonly \
    -modcacherw \
    -trimpath \
    -o /usr/bin/yap \
    cmd/yap/main.go

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
