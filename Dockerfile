# From here, until the final two RUN steps will be it's own Dockerfile which will be
# pre-built and put up in our private registry so while running specs, we will only
# run the final two RUN steps

FROM ubuntu

RUN echo "deb http://archive.ubuntu.com/ubuntu precise main universe" > /etc/apt/sources.list
RUN apt-get update

# Install global dependencies
RUN apt-get install -yy \
  aufs-tools \
  autoconf \
  bridge-utils \
  build-essential \
  bzr \
  ca-certificates \
  curl \
  git-core \
  iptables \
  lxc \
  mercurial \
  net-tools \
  ntp \
  quotatool \
  redis-server \
  s3cmd \
  tar \
  util-linux \
  wget

RUN curl -s https://go.googlecode.com/files/go1.2.linux-amd64.tar.gz | tar -v -C /usr/local -xz

RUN mkdir -p /opt/go \
  && mkdir -p /opt/go/bin \
  && mkdir -p /opt/go/src/github.com/action-io/goboxer

# mkfs requires /etc/mtab to be present
RUN touch /etc/mtab

ENV PATH /usr/local/go/bin:$PATH
ENV GOPATH /opt/go
ENV GOBIN /opt/go/bin
WORKDIR /opt/go/src/github.com/action-io/goboxer
