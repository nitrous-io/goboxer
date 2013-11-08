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
  # install a newer kernel
  apt-get install -q -y linux-image-generic-lts-raring linux-headers-generic-lts-raring
  apt-get install -yy build-essential
  apt-get install -yy git-core lxc bridge-utils curl redis-server build-essential xfsprogs lvm2 iptables aufs-tools mercurial
  apt-get install -yy quotatool thin-provisioning-tools

  # support for cgroup
  sed -i "s/GRUB_CMDLINE_LINUX=\"\"/GRUB_CMDLINE_LINUX=\"cgroup_enable=memory swapaccount=1\"/g" /etc/default/grub
  update-grub

  if [[ ! -f /usr/local/go/bin/go ]]; then
    # install go
    curl -s https://go.googlecode.com/files/go1.1.2.linux-amd64.tar.gz | tar -v -C /usr/local -xz
    echo "export PATH=/usr/local/go/bin:/usr/local/bin:/usr/local/sbin:/usr/bin:/usr/sbin:/bin:/sbin" >> /etc/profile
    echo "export GOPATH=/home/vagrant/gocode" >> /etc/profile
  fi

  cd $HOME
  mkdir -p $HOME/gocode/src/github.com/action-io
  ln -fs $HOME/gocode/src/github.com/action-io/goboxer $HOME/goboxer

  cd $HOME/gocode/src/github.com/action-io/goboxer
  ./script/setup
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
  config.vm.synced_folder "./", "/home/vagrant/gocode/src/github.com/action-io/goboxer"

  # Provider-specific configuration so you can fine-tune various
  # backing providers for Vagrant. These expose provider-specific options.
  # Example for VirtualBox:
  #
  config.vm.provider :virtualbox do |v|
    v.customize ["modifyvm", :id, "--cpus", "2"]
    v.customize ["modifyvm", :id, "--memory", "2048"]
    v.customize ["modifyvm", :id, "--natdnshostresolver1", "on"]
    v.customize ["modifyvm", :id, "--natdnsproxy1", "on"]
  end

  # View the documentation for the provider you're using for more
  # information on available options.
  # config.ssh.max_tries = 40
  # config.ssh.timeout   = 120
  config.ssh.forward_agent = true

  config.vm.provision "shell", inline: $script
end
