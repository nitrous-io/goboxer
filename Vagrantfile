IP = ENV['GOBOXER_INTERFACE'] || ENV['VAGRANT_INTERFACE'] || '33.33.33.11'

$script = <<SCRIPT
  export HOME=/home/vagrant
  apt-get update
  apt-get install -yy python-software-properties
  add-apt-repository http://ppa.launchpad.net/brightbox/experimental/ubuntu

  apt-get update
  # install a newer kernel
  apt-get install -q -y linux-image-generic-lts-raring linux-headers-generic-lts-raring
  apt-get install -yy build-essential
  apt-get install -yy git-core bzr lxc bridge-utils curl redis-server build-essential xfsprogs lvm2 iptables aufs-tools mercurial
  apt-get install -yy quotatool thin-provisioning-tools
  apt-get install -yy s3cmd

  # support for cgroup
  sed -i 's/GRUB_CMDLINE_LINUX=""/GRUB_CMDLINE_LINUX="cgroup_enable=memory swapaccount=1"/g' /etc/default/grub
  update-grub

  install_go() {
    curl -s https://go.googlecode.com/files/go1.2.linux-amd64.tar.gz | tar -v -C /usr/local -xz
  }

  if [[ ! -f /usr/local/go/bin/go ]]; then
    install_go
    echo "export PATH=/usr/local/go/bin:/usr/local/bin:/usr/local/sbin:/usr/bin:/usr/sbin:/bin:/sbin" >> /etc/profile
    echo "export GOPATH=/home/vagrant/gocode" >> /etc/profile
  else
    if [[ ! $(go version | grep go1.2) ]]; then
      install_go
      rm -rf $HOME/gocode/pkg
      rm -rf $HOME/gocode/bin
    fi
  fi

  cd $HOME
  mkdir -p $HOME/gocode/src/github.com/action-io
  ln -fs $HOME/gocode/src/github.com/action-io/goboxer $HOME/goboxer
  cd $HOME/gocode/src/github.com/action-io/goboxer
  /usr/local/go/bin/go get
  /usr/local/go/bin/go get launchpad.net/gocheck
  chown -R vagrant:vagrant $HOME/gocode

  ./script/setup

  echo 'To deploy, you need to setup s3cmd credentials. Run `s3cmd --configure` inside the box to start'
SCRIPT

Vagrant.configure('2') do |config|
  config.vm.hostname = 'goboxer'
  config.vm.box      = 'linux312'
  #config.vm.box_url  = 'https://opscode-vm.s3.amazonaws.com/vagrant/opscode_ubuntu-12.04_chef-11.2.0.box'

  config.vm.network :private_network, ip: IP

  config.vm.synced_folder './', '/home/vagrant/gocode/src/github.com/action-io/goboxer'

  config.vm.provider :virtualbox do |v|
    v.name = "goboxer-linux312"
    v.customize ['modifyvm', :id, '--cpus', ENV['GOBOXER_VM_CPU'] || '2']
    v.customize ['modifyvm', :id, '--memory', ENV['GOBOXER_VM_MEM'] || '2048']
    v.customize ['modifyvm', :id, '--natdnshostresolver1', 'on']
    v.customize ['modifyvm', :id, '--natdnsproxy1', 'on']
  end

  config.ssh.forward_agent = true

  config.vm.provision 'shell', inline: $script
end
