.PHONY: build install test vet fmt fmt-check check hooks try

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

fmt:
	gofmt -w .

fmt-check:
	@files="$$(gofmt -l .)"; if [ -n "$$files" ]; then echo "gofmt needed (run: gofmt -w .):"; echo "$$files"; exit 1; fi

check: fmt-check vet test

hooks:
	git config core.hooksPath .githooks

# try: rebuild and launch the TUI against a throwaway HOME so manual testing
# never touches your real ~/.skillpack. Use TRY_HOME=$$HOME to test on real data.
TRY_HOME ?= /tmp/skillpack-try
try:
	mkdir -p $(TRY_HOME)
	go build -o $(TRY_HOME)/skillpack $(CMD)
	HOME=$(TRY_HOME) $(TRY_HOME)/skillpack tui
