.PHONY: build install test vet fmt-check check hooks

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

fmt-check:
	@files="$$(gofmt -l .)"; if [ -n "$$files" ]; then echo "gofmt needed (run: gofmt -w .):"; echo "$$files"; exit 1; fi

check: fmt-check vet test

hooks:
	git config core.hooksPath .githooks
