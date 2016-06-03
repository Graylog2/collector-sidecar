GO ?= go
GOFMT ?= gofmt

COLLECTOR_VERSION = $(shell grep CollectorVersion common/metadata.go | awk '{gsub(/"/, "", $$3); print $$3}')
ifeq ($(strip $(COLLECTOR_VERSION)),)
$(error COLLECTOR_VERSION is not set)
endif

TEST_SUITE = \
	github.com/Graylog2/collector-sidecar/backends/nxlog \
	github.com/Graylog2/collector-sidecar/common

all: clean misc build

misc: ## Build NXMock for testing collector-sidecar
	$(GO) build -o misc/nxmock/nxlog misc/nxmock/main.go

fmt: ## Run gofmt
	@GOFMT=$(GOFMT) sh ./format.sh

clean: ## Remove binaries
	@rm -rf build
	@rm -rf dist/cache
	@rm -rf dist/tmp-build
	@rm -rf dist/tmp-dest
	@rm -rf dist/pkg

deps: glide
	./glide install

glide:
ifeq ($(shell uname),Darwin)
	curl -L https://github.com/Masterminds/glide/releases/download/0.10.2/glide-0.10.2-darwin-amd64.zip -o glide.zip
	unzip glide.zip
	mv ./darwin-amd64/glide ./glide
	rm -fr ./darwin-amd64
	rm ./glide.zip
else
	curl -L https://github.com/Masterminds/glide/releases/download/0.10.2/glide-0.10.2-linux-amd64.zip -o glide.zip
	unzip glide.zip
	mv ./linux-amd64/glide ./glide
	rm -fr ./linux-amd64
	rm ./glide.zip
endif

test: ## Run tests
	$(GO) test -v $(TEST_SUITE)

build: ## Build collector-sidecar binary for local target system
	$(GO) build -v -i -o graylog-collector-sidecar

build-all: build-linux build-linux32 build-darwin build-windows build-windows32

build-linux: ## Build collector-sidecar binary for Linux
	@mkdir -p build/$(COLLECTOR_VERSION)/linux/amd64
	GOOS=linux GOARCH=amd64 $(GO) build -v -i -o build/$(COLLECTOR_VERSION)/linux/amd64/graylog-collector-sidecar

build-linux32: ## Build collector-sidecar binary for Linux 32bit
	@mkdir -p build/$(COLLECTOR_VERSION)/linux/386
	GOOS=linux GOARCH=386 $(GO) build -v -i -o build/$(COLLECTOR_VERSION)/linux/386/graylog-collector-sidecar

build-darwin: ## Build collector-sidecar binary for OSX
	@mkdir -p build/$(COLLECTOR_VERSION)/darwin/amd64
	GOOS=darwin GOARCH=amd64 $(GO) build -v -i -o build/$(COLLECTOR_VERSION)/darwin/amd64/graylog-collector-sidecar

build-windows: ## Build collector-sidecar binary for Windows
	@mkdir -p build/$(COLLECTOR_VERSION)/windows/amd64
	GOOS=windows GOARCH=amd64 $(GO) build -v -i -o build/$(COLLECTOR_VERSION)/windows/amd64/graylog-collector-sidecar.exe

build-windows32: ## Build collector-sidecar binary for Windows 32bit
	@mkdir -p build/$(COLLECTOR_VERSION)/windows/386
	GOOS=windows GOARCH=386 $(GO) build -v -i -o build/$(COLLECTOR_VERSION)/windows/386/graylog-collector-sidecar.exe

build-solaris: ## Build collector-sidecar binary for Solaris
	@mkdir -p build/$(COLLECTOR_VERSION)/solaris/amd64
	GOOS=solaris GOARCH=amd64 $(GO) build -v -i -o build/$(COLLECTOR_VERSION)/solaris/amd64/graylog-collector-sidecar

package-all: package-linux package-linux32 package-windows package-windows32

package-linux: ## Create Linux system package
	@fpm-cook clean dist/recipe.rb
	@rm -rf dist/cache dist/tmp-build dist/tmp-dest
	@fpm-cook -t deb package dist/recipe.rb
	@fpm-cook -t rpm package dist/recipe.rb

package-linux32: ## Create Linux system package for 32bit hosts
	@fpm-cook clean dist/recipe32.rb
	@rm -rf dist/cache dist/tmp-build dist/tmp-dest
	@fpm-cook -t deb package dist/recipe32.rb
	@fpm-cook -t rpm package dist/recipe32.rb

package-windows: ## Create Windows installer
	@makensis dist/recipe.nsi

package-windows32: ## Create Windows installer for 32bit s hosts
	@makensis dist/recipe32.nsi

help:
	@grep -E '^[a-zA-Z0-9_-]+:.*?## .*$$' $(MAKEFILE_LIST) | sort | awk 'BEGIN {FS = ":.*?## "}; {printf "\033[36m%-30s\033[0m %s\n", $$1, $$2}'

.DEFAULT_GOAL := help

.PHONY: all build build-all build-linux build-linux32 build-darwin build-windows build-windows32 misc fmt clean help package-linux package-windows
