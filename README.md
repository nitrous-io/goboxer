## Vagrant setup
- Requires 'vagrant-vbguest': `vagrant plugin install vagrant-vbguest`

## Setup development environment

- setup `GOBOXER_INTERFACE` env to a different ip than 33.33.33.10
- setup the vagrant machine: `vagrant up`
- glare at the screen angrily as the provisioning will take some time
- Login to the box: `vagrant ssh`
- If you havent setup s3cmd, now is a good time to do that
- change to goboxer directory, run `go get`, then `make test`. *Ideally* it should pass !!!
