PACKAGE := github.com/action-io/goboxer
RELEASE_VERSION := $(shell git tag | grep -E "v[0-9\.]+$$" | sort -nr | head -n 1)
BINRELEASE := goboxer-$(RELEASE_VERSION).tgz
RELEASE_DIR := $(GOPATH)/src/github.com/action-io/goboxer/releases

NO_COLOR=\033[0m
OK_COLOR=\033[32;01m
ERROR_COLOR=\033[31;01m
WARN_COLOR=\033[33;01m

test:
	@sudo GOPATH=${GOPATH} ./script/test

test_log:
	@sudo ./script/test 2> test.log

build:
	@mkdir -p bin/
	@./script/build

bin: test build

release: test
	@./script/release

.PHONY: test bin
