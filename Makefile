GO ?= go
GOFMT ?= gofmt
AWK ?= awk

include version.mk
ifeq ($(strip $(COLLECTOR_VERSION)),)
$(error COLLECTOR_VERSION is not set)
endif

targets = graylog-sidecar sidecar-collector build dist/cache dist/tmp-build dist/tmp-dest dist/pkg dist/collectors
dist_targets = vendor

GIT_REV=$(shell git rev-parse --short HEAD)
BUILD_OPTS = -ldflags "-s -X github.com/Graylog2/collector-sidecar/common.GitRevision=$(GIT_REV) -X github.com/Graylog2/collector-sidecar/common.CollectorVersion=$(COLLECTOR_VERSION) -X github.com/Graylog2/collector-sidecar/common.CollectorVersionSuffix=$(COLLECTOR_VERSION_SUFFIX)"

TEST_SUITE = \
	github.com/Graylog2/collector-sidecar/common

all: build

fmt: ## Run gofmt
	@GOFMT=$(GOFMT) sh ./format.sh

clean: ## Remove binaries
	-rm -rf $(targets)

distclean: clean
	-rm -rf $(dist_targets)

test: ## Run tests
	$(GO) test -v $(TEST_SUITE)

build: ## Build sidecar binary for local target system
	$(GO) build $(BUILD_OPTS) -v -i -o graylog-sidecar

# does not include build-darwin as that only runs with homebrew on a Mac
build-all: build-linux-armv7 build-linux build-linux32 build-windows build-windows32 build-darwin build-freebsd

build-linux: ## Build sidecar binary for Linux
	@mkdir -p build/$(COLLECTOR_VERSION)/linux/amd64
	GOOS=linux GOARCH=amd64 $(GO) build $(BUILD_OPTS) -v -i -o build/$(COLLECTOR_VERSION)/linux/amd64/graylog-sidecar

solaris-sigar-patch:
	# https://github.com/cloudfoundry/gosigar/pull/28
	@if [ ! -e vendor/github.com/cloudfoundry/gosigar/sigar_solaris.go ]; then \
		wget -O vendor/github.com/cloudfoundry/gosigar/sigar_solaris.go https://raw.githubusercontent.com/amitkris/gosigar/9fc0903125acd1a0dc7635f8670088339865bcd5/sigar_solaris.go; \
	fi

build-linux-armv7: ## Build sidecar binary for linux-armv7
	@mkdir -p build/$(COLLECTOR_VERSION)/linux/armv7
	GOOS=linux GOARCH=arm GOARM=7 $(GO) build $(BUILD_OPTS) -pkgdir $(GOPATH)/go_linux-armv7  -v -i -o build/$(COLLECTOR_VERSION)/linux/armv7/graylog-sidecar

build-solaris: solaris-sigar-patch ## Build sidecar binary for Solaris/OmniOS/Illumos
	@mkdir -p build/$(COLLECTOR_VERSION)/solaris/amd64
	GOOS=solaris GOARCH=amd64 $(GO) build $(BUILD_OPTS) -v -i -o build/$(COLLECTOR_VERSION)/solaris/amd64/graylog-sidecar

build-linux32: ## Build sidecar binary for Linux 32bit
	@mkdir -p build/$(COLLECTOR_VERSION)/linux/386
	GOOS=linux GOARCH=386 $(GO) build $(BUILD_OPTS) -pkgdir $(GOPATH)/go_linux32  -v -i -o build/$(COLLECTOR_VERSION)/linux/386/graylog-sidecar

build-darwin: ## Build sidecar binary for OSX
	@mkdir -p build/$(COLLECTOR_VERSION)/darwin/amd64
	GOOS=darwin GOARCH=amd64 $(GO) build $(BUILD_OPTS) -pkgdir $(GOPATH)/go_darwin -v -i -o build/$(COLLECTOR_VERSION)/darwin/amd64/graylog-sidecar

build-freebsd:
	@mkdir -p build/$(COLLECTOR_VERSION)/freebsd/amd64
	GOOS=freebsd GOARCH=amd64 $(GO) build $(BUILD_OPTS) -pkgdir $(GOPATH)/go_freebsd -v -i -o build/$(COLLECTOR_VERSION)/freebsd/amd64/graylog-sidecar

build-windows: ## Build sidecar binary for Windows
	@mkdir -p build/$(COLLECTOR_VERSION)/windows/amd64
	go generate
	GOOS=windows GOARCH=amd64 CGO_ENABLED=1 CC=x86_64-w64-mingw32-gcc $(GO) build $(BUILD_OPTS) -pkgdir $(GOPATH)/go_win -v -i -o build/$(COLLECTOR_VERSION)/windows/amd64/graylog-sidecar.exe

build-windows32: ## Build sidecar binary for Windows 32bit
	@mkdir -p build/$(COLLECTOR_VERSION)/windows/386
	go generate
	GOOS=windows GOARCH=386 CGO_ENABLED=1 CC=i686-w64-mingw32-gcc $(GO) build $(BUILD_OPTS) -pkgdir $(GOPATH)/go_win32 -v -i -o build/$(COLLECTOR_VERSION)/windows/386/graylog-sidecar.exe

package-all: prepare-package package-linux-armv7 package-linux package-linux32 package-windows package-tar

prepare-package:
	dist/fetch_collectors.sh

package-linux-armv7: ## Create Linux ARMv7 system package
	fpm-cook clean dist/recipearmv7.rb
	rm -rf dist/cache dist/tmp-build dist/tmp-dest
	fpm-cook -t deb package dist/recipearmv7.rb
	fpm-cook -t rpm package dist/recipearmv7.rb

package-linux: ## Create Linux amd64 system package
	fpm-cook clean dist/recipe.rb
	rm -rf dist/cache dist/tmp-build dist/tmp-dest
	fpm-cook -t deb package dist/recipe.rb
	fpm-cook -t rpm package dist/recipe.rb

package-linux32: ## Create Linux i386 system package
	fpm-cook clean dist/recipe32.rb
	rm -rf dist/cache dist/tmp-build dist/tmp-dest
	fpm-cook -t deb package dist/recipe32.rb
	fpm-cook -t rpm package dist/recipe32.rb

package-windows: prepare-package ## Create Windows installer
	@mkdir -p dist/pkg
	makensis -DVERSION=$(COLLECTOR_VERSION) -DVERSION_SUFFIX=$(COLLECTOR_VERSION_SUFFIX) -DREVISION=$(COLLECTOR_REVISION) dist/recipe.nsi
	dist/chocolatey/gensha.sh $(COLLECTOR_VERSION) $(COLLECTOR_REVISION) $(COLLECTOR_VERSION_SUFFIX)

package-tar: ## Create tar archive for all platforms
	@mkdir -p dist/pkg
	@tar --transform="s|/build|/graylog-sidecar|" -Pczf dist/pkg/graylog-sidecar-$(COLLECTOR_VERSION)$(COLLECTOR_VERSION_SUFFIX).tar.gz ./build ./sidecar-example.yml ./sidecar-windows-example.yml

help:
	@grep -hE '^[a-zA-Z0-9_-]+:.*?## .*$$' $(MAKEFILE_LIST) | sort | $(AWK) 'BEGIN {FS = ":.*?## "}; {printf "\033[36m%-30s\033[0m %s\n", $$1, $$2}'

.DEFAULT_GOAL := all

.PHONY: all build build-all build-linux build-linux32 build-darwin build-freebsd build-windows build-windows32 fmt clean distclean help package-all package-linux package-linux32 package-windows package-tar
