## Setup development environment

go get -v github.com/action-io/goboxer/...

## Dependencies

- lxc
- libuv
- redigo
- gocheck

## TODO
- add configure
- change to use multidisk
- perhaps simplify the structure a.k.a merge boxer and commands
- init boxer steps in init()
- move boxer_test to integration test with redis etc.
- test stop/restart/destroy
