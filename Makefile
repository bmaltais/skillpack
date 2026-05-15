.PHONY: build install test vet

BINARY := skillpack
INSTALL_DIR := $(HOME)/.local/bin
CMD := ./cmd/skillpack/

build:
	go build -o $(BINARY) $(CMD)

install:
	go build -a -o $(INSTALL_DIR)/$(BINARY) $(CMD)

test:
	go test ./...

vet:
	go vet ./...

check: vet test
