# change this to correct package
SRC_DIR := $(GOPATH)/src
BUILD_DIR := $(CURDIR)/.gopath

BIN_RELATIVE := bin/goboxer
PKG := github.com/action-io/goboxer
PKG_DIR := $(SRC_DIR)/$(PKG)
PKG_MAIN := $(PKG_DIR)/goboxer
PKG_BIN := $(CURDIR)/$(BIN_RELATIVE)

GO_OPTIONS ?=
ifeq ($(VERBOSE), 1)
GO_OPTIONS += -v
endif

GIT_COMMIT = $(shell git rev-parse --short HEAD)
GIT_STATUS = $(shell test -n "`git status --porcelain`" && echo "+CHANGES")
BUILD_OPTIONS = -ldflags "-X main.GIT_COMMIT $(GIT_COMMIT)$(GIT_STATUS)"

.PHONY: all clean $(PKG_BIN) $(PKG_DIR)

all: $(PKG_BIN)

$(PKG_BIN): $(PKG_DIR)
	@mkdir -p  $(dir $@)
	@(cd $(PKG_MAIN); go build $(GO_OPTIONS) $(BUILD_OPTIONS) -o $@)
	@echo $(BIN_RELATIVE) is created.

$(PKG_DIR):
	@mkdir -p $(dir $@)
	@if [ -h $@ ]; then rm -f $@; fi; ln -sf $(CURDIR)/ $@
	@(cd $(PKG_MAIN); go get -d $(GO_OPTIONS))

clean:
	@rm -rf $(dir $(PKG_BIN))
ifeq ($(GOPATH), $(BUILD_DIR))
	@rm -rf $(BUILD_DIR)
else ifneq ($(PKG_DIR), $(realpath $(PKG_DIR)))
	@rm -f $(PKG_DIR)
endif

test:
	sudo -E go test -v github.com/action-io/goboxer/...
	#@(cd $(PKG_DIR); sudo -E go test $(GO_OPTIONS))

bin:
	go get -v github.com/action-io/goboxer/...
	go install -v github.com/action-io/goboxer/...
