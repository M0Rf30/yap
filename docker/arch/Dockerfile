FROM archlinux/base
MAINTAINER Yap <contact@packagefoundation.org>

RUN pacman -Syu --noconfirm
RUN pacman -S --noconfirm base-devel go git mercurial wget rsync tar zip sudo
RUN ln -s -f /usr/bin/pinentry-curses /usr/bin/pinentry

ENV GOPATH /go
ENV PATH $PATH:/go/bin

RUN go get github.com/packagefoundation/yap

RUN sed -i 's|bsdtar -xf "$dbfile" -C "$tmpdir/$repo"|tar -xf "$dbfile" -C "$tmpdir/$repo"|g' /usr/bin/repo-add

ENTRYPOINT ["yap"]
CMD ["build", "archlinux"]
