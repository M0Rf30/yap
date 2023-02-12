FROM oraclelinux:8
LABEL org.opencontainers.image.title="yap"
LABEL org.opencontainers.image.description="Package software with ease 📦"
LABEL org.opencontainers.image.vendor="M0Rf30"

SHELL ["/bin/bash", "-o", "pipefail", "-c"]
RUN \
  dnf -y install https://dl.fedoraproject.org/pub/epel/epel-release-latest-8.noarch.rpm; \
  dnf -y install dnf-plugins-core epel-release; \
  dnf config-manager --set-enabled ol8_addons; \
  dnf config-manager --set-enabled ol8_appstream; \
  dnf config-manager --set-enabled ol8_codeready_builder; \
  dnf -y groupinstall 'Development Tools'; \
  dnf -y install \
  automake bash-completion createrepo expect gcc git make openssl rpm-build rpm-sign rsync sudo tar zip; \
  curl -sL https://dl.google.com/go/go1.20.linux-amd64.tar.gz --output go.tar.gz; \
  echo "5a9ebcc65c1cce56e0d2dc616aff4c4cedcfbda8cc6f0288cc08cda3b18dcbf1 go.tar.gz" | sha256sum -c - ; \
  tar -C /usr/lib -xf go.tar.gz; \
  rm -f go.tar.gz; \
  ln -sf /usr/lib/go/bin/go /usr/bin/go; \
  ln -sf /usr/lib/go/bin/gofmt /usr/bin/gofmt; \
  ln -sf /usr/share/doc/go /usr/lib/go/doc; \
  dnf clean all

ENV GOROOT=/usr/lib/go
ENV PATH /usr/lib/go/bin:$PATH:/go/bin

COPY . /project
WORKDIR /project
RUN \
  go build -ldflags "-w -s" -trimpath -mod=readonly -modcacherw -buildmode=pie -o /usr/bin/; \
  yap completion bash > /etc/bash_completion.d/yap; \
  echo "source /usr/share/bash-completion/bash_completion" >> /etc/bashrc; \
  rm -rf /project

ENTRYPOINT ["yap"]
