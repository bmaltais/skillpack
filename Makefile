.PHONY: build install test vet check hooks

BINARY := skillpack
INSTALL_DIR := $(HOME)/.local/bin
CMD := ./cmd/skillpack/

build:
	go build -o $(BINARY) $(CMD)

install:
	mkdir -p $(INSTALL_DIR)
	go build -a -o $(INSTALL_DIR)/$(BINARY) $(CMD)

test:
	go test ./...

vet:
	go vet ./...

check: vet test

hooks:
	git config core.hooksPath .githooks
