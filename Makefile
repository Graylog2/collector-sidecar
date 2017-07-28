GO ?= go
GOFMT ?= gofmt
AWK ?= awk

include version.mk
ifeq ($(strip $(COLLECTOR_VERSION)),)
$(error COLLECTOR_VERSION is not set)
endif

GIT_REV=$(shell git rev-parse --short HEAD)
BUILD_OPTS = -ldflags "-s -X github.com/Graylog2/collector-sidecar/common.GitRevision=$(GIT_REV) -X github.com/Graylog2/collector-sidecar/common.CollectorVersion=$(COLLECTOR_VERSION) -X github.com/Graylog2/collector-sidecar/common.CollectorVersionSuffix=$(COLLECTOR_VERSION_SUFFIX)"

TEST_SUITE = \
	github.com/Graylog2/collector-sidecar/backends/nxlog \
	github.com/Graylog2/collector-sidecar/backends/beats \
	github.com/Graylog2/collector-sidecar/backends/beats/filebeat \
	github.com/Graylog2/collector-sidecar/backends/beats/winlogbeat \
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
	@rm -rf dist/collectors

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
	$(GO) build $(BUILD_OPTS) -v -i -o graylog-collector-sidecar

build-all: build-linux build-linux32 build-darwin build-windows build-windows32

build-linux: ## Build collector-sidecar binary for Linux
	@mkdir -p build/$(COLLECTOR_VERSION)/linux/amd64
	GOOS=linux GOARCH=amd64 $(GO) build $(BUILD_OPTS) -v -i -o build/$(COLLECTOR_VERSION)/linux/amd64/graylog-collector-sidecar

solaris-sigar-patch:
	# https://github.com/cloudfoundry/gosigar/pull/28
	@if [ ! -e vendor/github.com/cloudfoundry/gosigar/sigar_solaris.go ]; then \
		wget -O vendor/github.com/cloudfoundry/gosigar/sigar_solaris.go https://raw.githubusercontent.com/amitkris/gosigar/9fc0903125acd1a0dc7635f8670088339865bcd5/sigar_solaris.go; \
	fi

build-solaris: solaris-sigar-patch ## Build collector-sidecar binary for Solaris/OmniOS/Illumos
	@mkdir -p build/$(COLLECTOR_VERSION)/solaris/amd64
	GOOS=solaris GOARCH=amd64 $(GO) build $(BUILD_OPTS) -v -i -o build/$(COLLECTOR_VERSION)/solaris/amd64/graylog-collector-sidecar

build-linux32: ## Build collector-sidecar binary for Linux 32bit
	@mkdir -p build/$(COLLECTOR_VERSION)/linux/386
	GOOS=linux GOARCH=386 $(GO) build $(BUILD_OPTS) -pkgdir $(GOPATH)/go_linux32  -v -i -o build/$(COLLECTOR_VERSION)/linux/386/graylog-collector-sidecar

build-darwin: ## Build collector-sidecar binary for OSX
	@mkdir -p build/$(COLLECTOR_VERSION)/darwin/amd64
	GOOS=darwin GOARCH=amd64 $(GO) build $(BUILD_OPTS) -v -i -o build/$(COLLECTOR_VERSION)/darwin/amd64/graylog-collector-sidecar

build-freebsd:
	@mkdir -p build/$(COLLECTOR_VERSION)/freebsd/amd64
	GOOS=freebsd GOARCH=amd64 $(GO) build $(BUILD_OPTS) -v -i -o build/$(COLLECTOR_VERSION)/freebsd/amd64/graylog-collector-sidecar

build-windows: ## Build collector-sidecar binary for Windows
	@mkdir -p build/$(COLLECTOR_VERSION)/windows/amd64
	GOOS=windows GOARCH=amd64 CGO_ENABLED=1 CC=x86_64-w64-mingw32-gcc $(GO) build $(BUILD_OPTS) -pkgdir $(GOPATH)/go_win -v -i -o build/$(COLLECTOR_VERSION)/windows/amd64/graylog-collector-sidecar.exe

build-windows32: ## Build collector-sidecar binary for Windows 32bit
	@mkdir -p build/$(COLLECTOR_VERSION)/windows/386
	GOOS=windows GOARCH=386 CGO_ENABLED=1 CC=i686-w64-mingw32-gcc $(GO) build $(BUILD_OPTS) -pkgdir $(GOPATH)/go_win32 -v -i -o build/$(COLLECTOR_VERSION)/windows/386/graylog-collector-sidecar.exe

package-all: prepare-package package-linux package-linux32 package-windows package-tar

prepare-package:
	@dist/fetch_collectors.sh

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
	@mkdir -p dist/pkg
	@makensis -DVERSION=$(COLLECTOR_VERSION) -DVERSION_SUFFIX=$(COLLECTOR_VERSION_SUFFIX) -DREVISION=$(COLLECTOR_REVISION) dist/recipe.nsi

package-tar: ## Create tar archive for all platforms
	@mkdir -p dist/pkg
	@tar --transform="s|/build|/collector-sidecar|" -Pczf dist/pkg/collector-sidecar-$(COLLECTOR_VERSION)$(COLLECTOR_VERSION_SUFFIX).tar.gz ./build

help:
	@grep -E '^[a-zA-Z0-9_-]+:.*?## .*$$' $(MAKEFILE_LIST) | sort | $(AWK) 'BEGIN {FS = ":.*?## "}; {printf "\033[36m%-30s\033[0m %s\n", $$1, $$2}'

.DEFAULT_GOAL := help

.PHONY: all build build-all build-linux build-linux32 build-darwin build-windows build-windows32 misc fmt clean help package-linux package-windows
