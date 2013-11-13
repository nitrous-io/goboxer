## Vagrant setup
- Requires 'vagrant-vbguest'
```
vagrant plugin install vagrant-vbguest
```

## Setup development environment

- Get go1.1.1 from golang.org as ubuntu oneiric version seems to be outdated
  ```
  mkdir -p ~/golang-src

  cd ~/golang-src

  wget https://go.googlecode.com/files/go1.1.1.linux-amd64.tar.gz

  tar -xzf go1.1.1.linux-amd64.tar.gz

  sudo mv go /usr/local
  ```
- add /usr/local/go/bin into your PATH
  ```
  export PATH=$PATH:/usr/local/go/bin
  ```
- go get -v github.com/action-io/goboxer/...

## Dependencies

- go version go1.1.1 linux/amd64
- lxc
- redigo
- gocheck
