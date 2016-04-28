GO ?= go
GOFMT ?= gofmt

all: clean misc build

build: ## Build collector-sidecar binary for local target system
	$(GO) build -v -i -o graylog-collector-sidecar

build-linux: ## Build collector-sidecar binary for Linux
	GOOS=linux GOARCH=amd64 $(GO) build -v -i -o graylog-collector-sidecar

build-darwin: ## Build collector-sidecar binary for OSX
	GOOS=darwin GOARCH=amd64 $(GO) build -v -i -o graylog-collector-sidecar

build-windows: ## Build collector-sidecar binary for Windows
	GOOS=windows GOARCH=amd64 $(GO) build -v -i -o graylog-collector-sidecar.exe

misc: ## Build NXMock for testing collector-sidecar
	$(GO) build -o misc/nxmock/nxlog misc/nxmock/main.go

fmt: ## Run gofmt
	@GOFMT=$(GOFMT) sh ./format.sh

clean: ## Remove binaries
	rm -f graylog-collector-sidecar graylog-collector-sidecar.exe

package-linux: ## Create Linux system package
	@rm -f dist/pkg/graylog-collector-sidecar*
	@fpm-cook clean dist/recipe.rb
	@fpm-cook -t deb package dist/recipe.rb
	@fpm-cook -t rpm package dist/recipe.rb

package-windows: ## Create Windows installer
	@rm -f dist/graylog_collector_sidecar_installer.exe
	@makensis dist/recipe.nsi

help:
	@grep -E '^[a-zA-Z_-]+:.*?## .*$$' $(MAKEFILE_LIST) | sort | awk 'BEGIN {FS = ":.*?## "}; {printf "\033[36m%-30s\033[0m %s\n", $$1, $$2}'

.DEFAULT_GOAL := help

.PHONY: all build build-darwin build-windows misc clean help format
