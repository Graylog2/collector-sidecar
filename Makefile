GO ?= go

all: clean build

build: ## Build sidecar binary for local target system
	$(GO) build -v -i -o sidecar

build-linux: ## Build sidecar binary for Linux
	GOOS=linux GOARCH=amd64 $(GO) build -v -i -o sidecar

build-darwin: ## Build sidecar binary for OSX
	GOOS=darwin GOARCH=amd64 $(GO) build -v -i -o sidecar

build-windows: ## Build sidecar binary for Windows
	GOOS=windows GOARCH=amd64 $(GO) build -v -i -o sidecar.exe

misc: ## Build NXMock for testing sidecar
	$(GO) build -o misc/nxmock/nxlog misc/nxmock/main.go

fmt: ## Run gofmt
	$(GO) fmt

clean: ## Remove binaries
	rm -f sidecar sidecar.exe 

help:
	@grep -E '^[a-zA-Z_-]+:.*?## .*$$' $(MAKEFILE_LIST) | sort | awk 'BEGIN {FS = ":.*?## "}; {printf "\033[36m%-30s\033[0m %s\n", $$1, $$2}'

.DEFAULT_GOAL := help

.PHONY: all build build-darwin build-windows misc clean help
