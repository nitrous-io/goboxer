# -*- mode: ruby -*-
# vi: set ft=ruby :
#
IP = ENV['VAGRANT_INTERFACE'] || "33.33.33.10"

$script = <<SCRIPT
  export HOME=/home/vagrant
  apt-get update
  apt-get install -yy python-software-properties
  add-apt-repository http://ppa.launchpad.net/brightbox/experimental/ubuntu

  apt-get update
  apt-get install -yy build-essential
  apt-get install -yy git-core lxc bridge-utils curl redis-server build-essential xfsprogs lvm2 iptables aufs-tools mercurial
  apt-get install -yy quotatool thin-provisioning-tools

  if [[ ! -f /usr/local/go/bin/go ]]; then
    # install go
    curl -s https://go.googlecode.com/files/go1.1.2.linux-amd64.tar.gz | tar -v -C /usr/local -xz
    echo "export PATH=/usr/local/go/bin:/usr/local/bin:/usr/local/sbin:/usr/bin:/usr/sbin:/bin:/sbin" >> /etc/profile
    echo "export GOPATH=/home/vagrant/gocode" >> /etc/profile
  fi

  cd $HOME
  mkdir -p $HOME/gocode/src/github.com/action-io
  ln -fs $HOME/goboxer $HOME/gocode/src/github.com/action-io/goboxer

  # setup lvm
  if [[ ! -f $HOME/lxc_disk.img ]]; then
    dd if=/dev/zero of=$HOME/lxc_disk.img bs=1024 count=5000000
  fi
  if [[ ! `losetup -a | grep /dev/loop0`]]; then
    losetup /dev/loop0 $HOME/lxc_disk.img
  fi
  pvcreate /dev/loop0
  vgcreate aio_boxhost /dev/loop0
  lvcreate -L 4G --poolmetadatasize 200M --type thin-pool --thinpool boxes aio_boxhost

  # setup xfs
  if [[ ! -f $HOME/xfs_disk.img ]]; then
    dd if=/dev/zero of=$HOME/xfs_disk.img bs=1024 count=5000000
  fi
  if [[ ! `losetup -a | grep /dev/loop1`]]; then
    losetup /dev/loop1 $HOME/xfs_disk.img
  fi
  if [[ ! `blkid /dev/loop1 | grep xfs` ]]; then
    mkfs.xfs /dev/loop1
  fi
  mkdir -p /lxc/disk1
  umount /lxc/disk1
  mount -t xfs -o defaults,prjquota /dev/loop1 /lxc/disk1

  # setup ext4
  if [[ ! -f $HOME/ext4_disk.img ]]; then
    dd if=/dev/zero of=$HOME/ext4_disk.img bs=1024 count=5000000
  fi
  if [[ ! `losetup -a | grep /dev/loop2`]]; then
    losetup /dev/loop2 $HOME/ext4_disk.img
  fi
  if [[ ! `blkid /dev/loop1 | grep ext4` ]]; then
    mkfs.ext4 -i 8192 /dev/loop2
  fi
  mkdir -p /lxc/disk2
  umount /lxc/disk2
  mount -t ext4 -o defaults,usrquota /dev/loop2 /lxc/disk2
  if [[ ! -f /lxc/disk2/aquota.user ]]; then
    quotacheck -cu /lxc/disk2
    quotacheck -avu
    quotaon -av
  fi

  # install templates to all disks
  template=base_container_template
  if [[ ! -d /var/lib/lxc/${template} ]]; then
    lxc-create -n ${template} -t ubuntu -f $PWD/goboxer/script/lxc_default.conf -- -r precise
    chroot /var/lib/lxc/${template}/rootfs useradd action -m -s /bin/bash -d /home/action
    chroot /var/lib/lxc/base_container_template/rootfs rm -rf /dev/null
    chroot /var/lib/lxc/base_container_template/rootfs mknod -m 666 /dev/null c 1 3
  fi
  rm -rf /var/lib/lxc/test_container_template.tar*
  tar cvf /var/lib/lxc/test_container_template.tar --files-from /dev/null
  gzip /var/lib/lxc/test_container_template.tar

  # setup network interfaces
  if [[ ! `grep "br-lxc-254" /etc/network/interfaces` ]]; then
    echo -e "auto br-lxc-254\niface br-lxc-254 inet static\n\taddress 192.168.254.1\n\tnetmask 255.255.255.0\n\tpost-up echo 1 > /proc/sys/net/ipv4/ip_forward\n\tpre-down echo 0 > /proc/sys/net/ipv4/ip_forward\n\tbridge_ports none\n\tbridge_stp on\n\tbridge_fd 0\n\tbridge_maxwait 0" >> /etc/network/interfaces
  fi

  /sbin/ifconfig br-lxc-254 || /sbin/ifup br-lxc-254
  (/sbin/iptables -t nat --list | /bin/grep 192-168-254-0) || /sbin/iptables -t nat -A POSTROUTING -s 192.168.254.0/24 ! -d 192.168.254.0/24 -j MASQUERADE

  # install templates to all disks
  mkdir -p /lxc/disk1/base_container_template
  cp -af /var/lib/lxc/base_container_template /lxc/disk1/
  cp -af /var/lib/lxc/test_container_template.tar.gz /lxc/disk1/

  mkdir -p /lxc/disk2/base_container_template
  cp -af /var/lib/lxc/base_container_template /lxc/disk2/
  cp -af /var/lib/lxc/test_container_template.tar.gz /lxc/disk2/
SCRIPT

Vagrant.configure("2") do |config|
  # All Vagrant configuration is done here. The most common configuration
  # options are documented and commented below. For a complete reference,
  # please see the online documentation at vagrantup.com.

  config.vm.hostname = "goboxer"

  # Every Vagrant virtual environment requires a box to build off of.
  config.vm.box      = 'opscode_ubuntu-12.04_chef-11.2.0'

  # The url from where the 'config.vm.box' box will be fetched if it
  # doesn't already exist on the user's system.
  config.vm.box_url  = "https://opscode-vm.s3.amazonaws.com/vagrant/opscode_ubuntu-12.04_chef-11.2.0.box"

  # Assign this VM to a host-only network IP, allowing you to access it
  # via the IP. Host-only networks can talk to the host machine as well as
  # any other machines on the same network, but cannot be accessed (through this
  # network interface) by any external networks.
  config.vm.network :private_network, ip: IP

  # Share an additional folder to the guest VM. The first argument is
  # the path on the host to the actual folder. The second argument is
  # the path on the guest to mount the folder. And the optional third
  # argument is a set of non-required options.
  config.vm.synced_folder "./", "/home/vagrant/goboxer"

  # Provider-specific configuration so you can fine-tune various
  # backing providers for Vagrant. These expose provider-specific options.
  # Example for VirtualBox:
  #
  config.vm.provider :virtualbox do |v|
    v.customize ["modifyvm", :id, "--cpus", "2"]
    v.customize ["modifyvm", :id, "--memory", "2048"]
    v.customize ["modifyvm", :id, "--natdnshostresolver1", "on"]
  end

  # View the documentation for the provider you're using for more
  # information on available options.
  # config.ssh.max_tries = 40
  # config.ssh.timeout   = 120

  config.vm.provision "shell", inline: $script
end
