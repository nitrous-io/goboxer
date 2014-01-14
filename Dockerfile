# From here, until the final two RUN steps will be it's own Dockerfile which will be
# pre-built and put up in our private registry so while running specs, we will only
# run the final two RUN steps

FROM ubuntu

ENV REDIS_VERSION 2.6.12
ENV REDIS_PORT 65000
ENV REDIS_USERNAME redis

ENV PATH /usr/local/go/bin:$PATH
ENV GOBOXER_USERNAME app_runner
ENV GOPATH /home/${GOBOXER_USERNAME}/gocode


# Set working directory to /var/apps/goboxer (that is where the app will be built from)
WORKDIR /var/apps/goboxer

RUN echo "deb http://archive.ubuntu.com/ubuntu precise main universe" > /etc/apt/sources.list
RUN apt-get update

# Install global dependencies
RUN apt-get install -yy build-essential autoconf curl wget git-core util-linux ntp tar

# Install goboxer dependencies

RUN apt-get install -yy python-software-properties
RUN add-apt-repository http://ppa.launchpad.net/brightbox/experimental/ubuntu
RUN apt-get update

RUN curl -s https://go.googlecode.com/files/go1.2.linux-amd64.tar.gz | tar -v -C /usr/local -xz

# Install Redis dependencies

RUN apt-get install -yy aufs-tools bridge-utils bzr redis-server xfsprogs lvm2 mercurial quotatool thin-provisioning-tools s3cmd

# Create goboxer user

RUN useradd -U -m $GOBOXER_USERNAME
RUN mkdir /home/$GOBOXER_USERNAME/gocode
RUN chown $GOBOXER_USERNAME: /home/$GOBOXER_USERNAME/gocode
RUN mkdir -p /home/$GOBOXER_USERNAME/gocode/src/github.com/action-io

# "Mount" the current directory in /var/apps/goboxer
ADD ./ /home/$GOBOXER_USERNAME/gocode/src/github.com/action-io/goboxer

# Disable udev for LVM:

RUN sed -i 's/udev_sync = 1/udev_sync = 0/' /etc/lvm/lvm.conf
RUN sed -i 's/udev_rules = 1/udev_rules = 0/' /etc/lvm/lvm.conf

RUN \
   ln -fs /home/$GOBOXER_USERNAME/gocode/src/github.com/action-io/goboxer /home/$GOBOXER_USERNAME/goboxer &&\
   cd /home/$GOBOXER_USERNAME/gocode/src/github.com/action-io/goboxer &&\
   /usr/local/go/bin/go get &&\
   /usr/local/go/bin/go get launchpad.net/gocheck &&\
   chown -R $GOBOXER_USERNAME: /home/$GOBOXER_USERNAME/gocode &&\
   ./script/setup
