# Goboxer

goboxer is a service which provisions boxes (LXC containers).
It listens for messages on a Redis pub/sub channel and performs the requisite
operations (provisioning, setting up iptables filters, IPs, etc.) to setup a
working dev environment.

goboxer is written in Go and deprecates [Boxer](https://github.com/action-io/boxer).

## Development

Developing goboxer is best done in a VM.

1. Install [Vagrant](http://downloads.vagrantup.com/).
1. Install the [vagrant-berkshelf plugin](https://github.com/riotgames/vagrant-berkshelf): `vagrant plugin install vagrant-berkshelf`.
1. Install the [vagrant-vbguest plugin](https://github.com/dotless-de/vagrant-vbguest): `vagrant plugin install vagrant-vbguest`.
1. Install [VirtualBox](https://www.virtualbox.org/wiki/Downloads).
1. You might want to set the `GOBOXER_INTERFACE` env var to a different IP than
   the default.
1. `vagrant up`
1. Glare at the screen angrily as the provisioning will take some time.
1. Login to the box: `vagrant ssh`.
1. If you haven't setup s3cmd, now is a good time to do that: `s3cmd --configure`.

### Git setup

Since this is a new VM, don't forget to tell Git your name and email:

```
git config --global user.name "Your Name Here"
git config --global user.email "me@example.com"
```

You might want to setup your dotfiles too since this is a new development environment.

### Running tests

1. Change to goboxer directory: `cd $HOME/goboxer`.
1. Install dependencies: `go get` and `go get launchpad.net/gocheck`.
1. `make test`. *Ideally* it should pass!!!

#### Running individual tests

Tests are implemented with [gocheck](http://labix.org/gocheck).

1. Run specific tests with: `go test -gocheck.v -gocheck.f TestInvalidateCache`.
1. Some tests, like those in `boxer_test.go`, require sudo: `sudo -E go test -gocheck.v -gocheck.f TestCreateBox`.

### Releasing a new version of goboxer

1. You'll need to set your git `user.name` and `user.email` first.
1. Setup s3cmd as well if you haven't: `s3cmd --configure`.
1. `cd $HOME/goboxer`
1. Tag a new release and upload the build to S3 with: `make release`
