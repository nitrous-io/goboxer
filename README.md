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

- Furthermore, this requires docker to be in your root path. The fastest way to
  add it is by changing /etc/sudoers to include $GOBIN in your secure path

## Dependencies

- go version go1.1.1 linux/amd64
- lxc
- redigo
- gocheck
