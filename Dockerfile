# From here, until the final two RUN steps will be it's own Dockerfile which will be
# pre-built and put up in our private registry so while running specs, we will only
# run the final two RUN steps

FROM ubuntu

ENV REDIS_VERSION 2.6.12
ENV REDIS_PORT 65000
ENV REDIS_USERNAME redis

ENV PATH /usr/local/go/bin:$PATH
ENV GOBOXER_USERNAME app_runner
ENV HOME /home/${GOBOXER_USERNAME}
ENV GOPATH ${HOME}/gocode


# Set working directory to /var/apps/goboxer (that is where the app will be built from)
WORKDIR /home/app_runner/goboxer

RUN echo "deb http://archive.ubuntu.com/ubuntu precise main universe" > /etc/apt/sources.list
RUN apt-get update

# Install global dependencies
RUN apt-get install -yy build-essential autoconf curl wget git-core util-linux ntp tar

# Install goboxer/redis dependencies

RUN apt-get install -yy python-software-properties
RUN add-apt-repository http://ppa.launchpad.net/brightbox/experimental/ubuntu
RUN apt-get update

RUN curl -s https://go.googlecode.com/files/go1.2.linux-amd64.tar.gz | tar -v -C /usr/local -xz

RUN apt-get install -yy aufs-tools bridge-utils bzr redis-server xfsprogs lvm2 mercurial lxc quotatool thin-provisioning-tools s3cmd iptables ca-certificates

# Create goboxer user

RUN useradd -U -m $GOBOXER_USERNAME
RUN mkdir ${HOME}/gocode
RUN chown $GOBOXER_USERNAME: ${HOME}/gocode
RUN mkdir -p ${HOME}/gocode/src/github.com/action-io

# Disable udev for LVM:

RUN sed -i 's/udev_sync = 1/udev_sync = 0/' /etc/lvm/lvm.conf
RUN sed -i 's/udev_rules = 1/udev_rules = 0/' /etc/lvm/lvm.conf

# Get disks made:

RUN dd if=/dev/zero of=$HOME/lxc_disk.img bs=1024 count=5000000
RUN dd if=/dev/zero of=$HOME/xfs_disk.img bs=1024 count=5000000
RUN dd if=/dev/zero of=$HOME/ext4_disk.img bs=1024 count=5000000

# Get wrapdocker inside:

ADD ./script/wrapdocker /usr/local/bin/wrapdocker
CMD ["wrapdocker"]

### MUTABLE BITS: THIS MUST BE AT THE END SO THE CACHE DOES NOT GET
### BUSTED!

VOLUME /var/lib/lxc

# "Mount" the goboxer directory into the container
ADD ./ ${HOME}/gocode/src/github.com/action-io/goboxer

# Prep the codebase

RUN \
   ln -fs ${HOME}/gocode/src/github.com/action-io/goboxer ${HOME}/goboxer &&\
   cd ${HOME}/gocode/src/github.com/action-io/goboxer &&\
   /usr/local/go/bin/go get &&\
   /usr/local/go/bin/go get launchpad.net/gocheck &&\
   chown -R $GOBOXER_USERNAME: ${HOME}/gocode
