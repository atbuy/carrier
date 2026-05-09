MODULE := github.com/user/carrier
BIN ?= bin/carrier

GO ?= go
GOFUMPT ?= gofumpt
GOIMPORTS_REVISER ?= goimports-reviser
GOLANGCI_LINT ?= golangci-lint
ZENSICAL ?= zensical
ARGS ?= --help

.PHONY: help fmt lint test build run clean docs-build docs-serve

help:
	@printf '%s\n' \
		'Targets:' \
		'  make fmt          Format Go files with gofumpt and goimports-reviser' \
		'  make lint         Run golangci-lint' \
		'  make test         Run Go tests' \
		'  make build        Build bin/carrier' \
		'  make run ARGS=... Run carrier locally' \
		'  make docs-build   Build Zensical docs into site/' \
		'  make docs-serve   Serve Zensical docs locally'

fmt:
	$(GOFUMPT) -w .
	$(GOIMPORTS_REVISER) -recursive -rm-unused -format -project-name $(MODULE) ./...

lint:
	$(GOLANGCI_LINT) run ./...

test:
	$(GO) test ./...

build:
	$(GO) build -o $(BIN) ./cmd/carrier

run:
	$(GO) run ./cmd/carrier $(ARGS)

clean:
	$(GO) clean
	rm -rf bin site

docs-build:
	$(ZENSICAL) build

docs-serve:
	$(ZENSICAL) serve
