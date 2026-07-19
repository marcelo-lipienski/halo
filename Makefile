# halo Makefile

MODULE      := github.com/marcelo-lipienski/halo
BINARY      := halo
VERSION     ?= $(shell git describe --tags --always --dirty 2>/dev/null || echo "dev")
COMMIT_SHA  ?= $(shell git rev-parse --short HEAD 2>/dev/null || echo "unknown")
LDFLAGS     := -ldflags "-X main.Version=$(VERSION) -X main.CommitSHA=$(COMMIT_SHA)"
GO          := go

.PHONY: all build install test bench lint vet fmt clean help

all: build

## build: Compile the binary with version/commit injection
build:
	$(GO) build $(LDFLAGS) -o $(BINARY) .

## install: Install the binary to GOPATH/bin with version injection
install:
	$(GO) install $(LDFLAGS) .

## test: Run the full test suite
test:
	$(GO) test ./... -count=1

## bench: Run benchmark tests
bench:
	$(GO) test ./... -bench=. -benchmem -run='^$$'

## vet: Run go vet
vet:
	$(GO) vet ./...

## fmt: Check formatting (non-destructive)
fmt:
	@gofmt -l . | tee /dev/stderr | xargs -r false

## fmt-fix: Apply gofmt formatting in place
fmt-fix:
	gofmt -w .

## lint: Run golangci-lint (requires golangci-lint to be installed)
lint:
	golangci-lint run ./...

## clean: Remove built binary and test artifacts
clean:
	rm -f $(BINARY) $(BINARY).exe *.test *.out *.log

## help: Display this help message
help:
	@echo "Available targets:"
	@grep -E '^## ' Makefile | sed 's/## /  /'
