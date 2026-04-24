GO ?= go
GOFMT ?= gofmt
AWK ?= awk

# Detect a GNU tar binary (prefer TAR env var, then tar, then gtar)
TAR ?= tar

# Is $(TAR) GNU?
GNU_TAR := $(shell $(TAR) --version 2>/dev/null | head -1 | grep -c 'GNU tar')

ifeq ($(GNU_TAR),0)
  # Try gtar
  TAR := gtar
  GNU_TAR := $(shell $(TAR) --version 2>/dev/null | head -1 | grep -c 'GNU tar')
endif

ifeq ($(GNU_TAR),0)
  $(error "GNU tar is required. On macOS: brew install gnu-tar")
endif

GOVERSIONINFO_BIN = $(shell go env GOPATH)/bin/goversioninfo

include version.mk
include branding.mk
# Load customization branding file if present, silently ignore otherwise
-include $(BRANDING_MK)

ifeq ($(strip $(COLLECTOR_VERSION)),)
$(error COLLECTOR_VERSION is not set)
endif

targets = $(BRAND_BINARY_NAME) sidecar-collector build dist/cache dist/tmp-build dist/tmp-dest dist/pkg dist/collectors resource_windows.syso dist/chocolatey/tools/chocolateyinstall.ps1 versioninfo.json
dist_targets = vendor

GIT_REV=$(shell git rev-parse --short HEAD)

# Branding ldflags
BRANDING_OPTS =
ifneq ($(strip $(BRAND_VENDOR_NAME)),)
BRANDING_OPTS += -X github.com/Graylog2/collector-sidecar/common.VendorName=$(BRAND_VENDOR_NAME)
endif
ifneq ($(strip $(BRAND_PRODUCT_NAME)),)
BRANDING_OPTS += -X github.com/Graylog2/collector-sidecar/common.ProductName=$(BRAND_PRODUCT_NAME)
endif

BUILD_OPTS = -ldflags "-s -X github.com/Graylog2/collector-sidecar/common.GitRevision=$(GIT_REV) -X github.com/Graylog2/collector-sidecar/common.CollectorVersion=$(COLLECTOR_VERSION) -X github.com/Graylog2/collector-sidecar/common.CollectorVersionSuffix=$(COLLECTOR_VERSION_SUFFIX) $(BRANDING_OPTS)"

TEST_SUITE = \
	github.com/Graylog2/collector-sidecar/common

WINDOWS_INSTALLER_VERSION = $(COLLECTOR_VERSION)-$(COLLECTOR_REVISION)$(subst -,.,$(COLLECTOR_VERSION_SUFFIX))
# Removing the dot to comply with NuGet versioning (beta.1 -> beta2)
CHOCOLATEY_VERSION = $(COLLECTOR_VERSION).$(COLLECTOR_REVISION)$(subst .,,$(COLLECTOR_VERSION_SUFFIX))

.PHONY: all
all: build

.PHONY: fmt
fmt: ## Run gofmt
	@GOFMT=$(GOFMT) sh ./format.sh

.PHONY: clean
clean: ## Remove binaries
	-rm -rf $(targets)

.PHONY: distclean
distclean: clean
	-rm -rf $(dist_targets)

.PHONY: test
test: ## Run tests
	$(GO) test -v $(TEST_SUITE)

.PHONY: build
build: ## Build sidecar binary for local target system
	$(GO) build $(BUILD_OPTS) -o $(BRAND_BINARY_NAME)

.PHONY: build-all
build-all: build-linux-amd64 build-linux-arm64 build-linux-armv7 build-linux32
build-all: build-darwin-amd64 build-darwin-arm64
build-all: build-freebsd-amd64
build-all: build-windows-amd64 build-windows32

.PHONY: build-linux-amd64
build-linux-amd64: ## Build sidecar binary for linux-amd64
	@mkdir -p build/$(COLLECTOR_VERSION)/linux/amd64
	GOOS=linux GOARCH=amd64 $(GO) build $(BUILD_OPTS) -o build/$(COLLECTOR_VERSION)/linux/amd64/$(BRAND_BINARY_NAME)

.PHONY: build-linux-arm64
build-linux-arm64: ## Build sidecar binary for linux-arm64
	@mkdir -p build/$(COLLECTOR_VERSION)/linux/arm64
	GOOS=linux GOARCH=arm64 $(GO) build $(BUILD_OPTS) -pkgdir $(GOPATH)/go_linux-arm64  -o build/$(COLLECTOR_VERSION)/linux/arm64/$(BRAND_BINARY_NAME)

.PHONY: build-linux-armv7
build-linux-armv7: ## Build sidecar binary for linux-armv7
	@mkdir -p build/$(COLLECTOR_VERSION)/linux/armv7
	GOOS=linux GOARCH=arm GOARM=7 $(GO) build $(BUILD_OPTS) -pkgdir $(GOPATH)/go_linux-armv7  -o build/$(COLLECTOR_VERSION)/linux/armv7/$(BRAND_BINARY_NAME)

.PHONY: build-linux32
build-linux32: ## Build sidecar binary for Linux 32bit
	@mkdir -p build/$(COLLECTOR_VERSION)/linux/386
	GOOS=linux GOARCH=386 $(GO) build $(BUILD_OPTS) -pkgdir $(GOPATH)/go_linux32  -o build/$(COLLECTOR_VERSION)/linux/386/$(BRAND_BINARY_NAME)

.PHONY: build-darwin-amd64
build-darwin-amd64: ## Build sidecar binary for OSX
	@mkdir -p build/$(COLLECTOR_VERSION)/darwin/amd64
	GOOS=darwin GOARCH=amd64 $(GO) build $(BUILD_OPTS) -pkgdir $(GOPATH)/go_darwin -o build/$(COLLECTOR_VERSION)/darwin/amd64/$(BRAND_BINARY_NAME)

.PHONY: build-darwin-arm64
build-darwin-arm64: ## Build sidecar binary for OSX
	@mkdir -p build/$(COLLECTOR_VERSION)/darwin/arm64
	GOOS=darwin GOARCH=arm64 $(GO) build $(BUILD_OPTS) -pkgdir $(GOPATH)/go_darwin-arm64 -o build/$(COLLECTOR_VERSION)/darwin/arm64/$(BRAND_BINARY_NAME)

.PHONY: build-freebsd-amd64
build-freebsd-amd64: ## Build sidecar binary for FreeBSD
	@mkdir -p build/$(COLLECTOR_VERSION)/freebsd/amd64
	GOOS=freebsd GOARCH=amd64 $(GO) build $(BUILD_OPTS) -pkgdir $(GOPATH)/go_freebsd -o build/$(COLLECTOR_VERSION)/freebsd/amd64/$(BRAND_BINARY_NAME)

.PHONY: build-windows-amd64
build-windows-amd64: install-goversioninfo versioninfo.json ## Build sidecar binary for Windows
	@mkdir -p build/$(COLLECTOR_VERSION)/windows/amd64
	$(GOVERSIONINFO_BIN) -64 -product-version="$(COLLECTOR_VERSION)-$(COLLECTOR_REVISION)" -ver-major="$(COLLECTOR_VERSION_MAJOR)" -product-ver-minor="$(COLLECTOR_VERSION_MINOR)" -product-ver-patch="$(COLLECTOR_VERSION_PATCH)" -product-ver-build="$(COLLECTOR_REVISION)" -file-version="$(COLLECTOR_VERSION)-$(COLLECTOR_REVISION)" -ver-major="$(COLLECTOR_VERSION_MAJOR)" -ver-minor="$(COLLECTOR_VERSION_MINOR)" -ver-patch="$(COLLECTOR_VERSION_PATCH)" -ver-build="$(COLLECTOR_REVISION)" -o resource_windows.syso
	GOOS=windows GOARCH=amd64 CGO_ENABLED=1 CC=x86_64-w64-mingw32-gcc $(GO) build $(BUILD_OPTS) -pkgdir $(GOPATH)/go_win -o build/$(COLLECTOR_VERSION)/windows/amd64/$(BRAND_BINARY_NAME).exe

.PHONY: build-windows32
build-windows32: install-goversioninfo versioninfo.json ## Build sidecar binary for Windows 32bit
	@mkdir -p build/$(COLLECTOR_VERSION)/windows/386
	$(GOVERSIONINFO_BIN) -product-version="$(COLLECTOR_VERSION)-$(COLLECTOR_REVISION)" -ver-major="$(COLLECTOR_VERSION_MAJOR)" -product-ver-minor="$(COLLECTOR_VERSION_MINOR)" -product-ver-patch="$(COLLECTOR_VERSION_PATCH)" -product-ver-build="$(COLLECTOR_REVISION)" -file-version="$(COLLECTOR_VERSION)-$(COLLECTOR_REVISION)" -ver-major="$(COLLECTOR_VERSION_MAJOR)" -ver-minor="$(COLLECTOR_VERSION_MINOR)" -ver-patch="$(COLLECTOR_VERSION_PATCH)" -ver-build="$(COLLECTOR_REVISION)" -o resource_windows.syso
	GOOS=windows GOARCH=386 CGO_ENABLED=1 CC=i686-w64-mingw32-gcc $(GO) build $(BUILD_OPTS) -pkgdir $(GOPATH)/go_win32 -o build/$(COLLECTOR_VERSION)/windows/386/$(BRAND_BINARY_NAME).exe

.PHONY: build-solaris
build-solaris: ## Build sidecar binary for Solaris/OmniOS/Illumos
	@mkdir -p build/$(COLLECTOR_VERSION)/solaris/amd64
	GOOS=solaris GOARCH=amd64 $(GO) build $(BUILD_OPTS) -o build/$(COLLECTOR_VERSION)/solaris/amd64/$(BRAND_BINARY_NAME)

.PHONY: sign-binaries
sign-binaries: sign-binary-windows-amd64 sign-binary-windows-386

.PHONY: sign-binary-windows-amd64
sign-binary-windows-amd64:
	# This needs to run in a Docker container with the graylog/internal-codesigntool image
	codesigntool sign build/$(COLLECTOR_VERSION)/windows/amd64/$(BRAND_BINARY_NAME).exe

.PHONY: sign-binary-windows-386
sign-binary-windows-386:
	# This needs to run in a Docker container with the graylog/internal-codesigntool image
	codesigntool sign build/$(COLLECTOR_VERSION)/windows/386/$(BRAND_BINARY_NAME).exe

## Adds version info to Windows executable
.PHONY: install-goversioninfo
install-goversioninfo:
	go install github.com/josephspurrier/goversioninfo/cmd/goversioninfo@latest

## Generate versioninfo.json from template with branding
versioninfo.json: versioninfo.json.template branding.mk
	sed -e 's|%%BRAND_VENDOR_DISPLAY%%|$(BRAND_VENDOR_DISPLAY)|g' \
	    -e 's|%%BRAND_PRODUCT_DISPLAY%%|$(BRAND_PRODUCT_DISPLAY)|g' \
	    -e 's|%%BRAND_ICON_FILE%%|$(BRAND_ICON_FILE_ABS)|g' \
	    $< > $@

.PHONY: package-all
package-all: prepare-package
package-all: package-linux-armv7 package-linux-arm64 package-linux-amd64 package-linux32
package-all: package-windows-exe-amd64 package-windows-msi-amd64
package-all: package-tar

.PHONY: prepare-package
prepare-package:
	dist/fetch_collectors.sh

# Export branding variables for FPM recipes
FPM_BRAND_ENV = \
	BRAND_VENDOR_NAME="$(BRAND_VENDOR_NAME)" \
	BRAND_VENDOR_DISPLAY="$(BRAND_VENDOR_DISPLAY)" \
	BRAND_VENDOR_LOWER="$(BRAND_VENDOR_LOWER)" \
	BRAND_PRODUCT_NAME="$(BRAND_PRODUCT_NAME)" \
	BRAND_PRODUCT_NAME_LOWER="$(BRAND_PRODUCT_NAME_LOWER)" \
	BRAND_PRODUCT_LOWER="$(BRAND_PRODUCT_LOWER)" \
	BRAND_HOMEPAGE_URL="$(BRAND_HOMEPAGE_URL)" \
	BRAND_MAINTAINER="$(BRAND_MAINTAINER)" \
	BRAND_LICENSE="$(BRAND_LICENSE)" \
	BRAND_CONFIG_DIR_UNIX="$(BRAND_CONFIG_DIR_UNIX)" \
	BRAND_LIB_DIR_UNIX="$(BRAND_LIB_DIR_UNIX)" \
	BRAND_LOG_DIR_UNIX="$(BRAND_LOG_DIR_UNIX)" \
	BRAND_CACHE_DIR_UNIX="$(BRAND_CACHE_DIR_UNIX)" \
	BRAND_VAR_LIB_DIR_UNIX="$(BRAND_VAR_LIB_DIR_UNIX)" \
	BRAND_VAR_RUN_DIR_UNIX="$(BRAND_VAR_RUN_DIR_UNIX)"

.PHONY: package-linux-armv7
package-linux-armv7: ## Create Linux ARMv7 system package
	$(FPM_BRAND_ENV) fpm-cook clean dist/recipearmv7.rb
	rm -rf dist/cache dist/tmp-build dist/tmp-dest
	$(FPM_BRAND_ENV) fpm-cook -t deb package dist/recipearmv7.rb
	$(FPM_BRAND_ENV) fpm-cook -t rpm package dist/recipearmv7.rb

.PHONY: package-linux-arm64
package-linux-arm64: ## Create Linux ARM64 system package
	$(FPM_BRAND_ENV) fpm-cook clean dist/recipearm64.rb
	rm -rf dist/cache dist/tmp-build dist/tmp-dest
	$(FPM_BRAND_ENV) fpm-cook -t deb package dist/recipearm64.rb
	$(FPM_BRAND_ENV) fpm-cook -t rpm package dist/recipearm64.rb

.PHONY: package-linux-amd64
package-linux-amd64: ## Create Linux amd64 system package
	$(FPM_BRAND_ENV) fpm-cook clean dist/recipe.rb
	rm -rf dist/cache dist/tmp-build dist/tmp-dest
	$(FPM_BRAND_ENV) fpm-cook -t deb package dist/recipe.rb
	$(FPM_BRAND_ENV) fpm-cook -t rpm package dist/recipe.rb

.PHONY: package-linux32
package-linux32: ## Create Linux i386 system package
	$(FPM_BRAND_ENV) fpm-cook clean dist/recipe32.rb
	rm -rf dist/cache dist/tmp-build dist/tmp-dest
	$(FPM_BRAND_ENV) fpm-cook -t deb package dist/recipe32.rb
	$(FPM_BRAND_ENV) fpm-cook -t rpm package dist/recipe32.rb

.PHONY: package-windows-exe-amd64
package-windows-exe-amd64: prepare-package ## Create Windows installer
	@mkdir -p dist/pkg
	makensis -DVERSION=$(COLLECTOR_VERSION) \
		-DVERSION_SUFFIX=$(COLLECTOR_VERSION_SUFFIX) \
		-DREVISION=$(COLLECTOR_REVISION) \
		-DBRAND_VENDOR_NAME="$(BRAND_VENDOR_NAME)" \
		-DBRAND_VENDOR_DISPLAY="$(BRAND_VENDOR_DISPLAY)" \
		-DBRAND_PRODUCT_NAME="$(BRAND_PRODUCT_NAME)" \
		-DBRAND_PRODUCT_DISPLAY="$(BRAND_PRODUCT_DISPLAY)" \
		-DBRAND_PRODUCT_LOWER="$(BRAND_PRODUCT_LOWER)" \
		-DBRAND_HOMEPAGE_URL="$(BRAND_HOMEPAGE_URL)" \
		-DBRAND_ICON_FILE="$(BRAND_ICON_FILE_ABS)" \
		-DBRAND_REGISTRY_KEY="$(BRAND_REGISTRY_KEY)" \
		-DBRAND_WIN_VENDOR_DIR="$(BRAND_WIN_VENDOR_DIR)" \
		-DBRAND_WIN_PRODUCT_DIR="$(BRAND_WIN_PRODUCT_DIR)" \
		dist/recipe.nsi


.PHONY: package-windows-msi-amd64
package-windows-msi-amd64: prepare-package ## Create Windows MSI package (requires packages: msitools, wixl)
	@mkdir -p dist/pkg
	wixl -v -a x64 \
		-D Version=$(COLLECTOR_VERSION)$(COLLECTOR_VERSION_SUFFIX) \
		-D LicensePath=LICENSE \
		-D SidecarEXEPath=build/$(COLLECTOR_VERSION)/windows/amd64/$(BRAND_BINARY_NAME).exe \
		-D SidecarConfigPath=sidecar-windows-msi-example.yml \
		-D FilebeatEXEPath=dist/collectors/filebeat/windows/x86_64/filebeat.exe \
		-D WinlogbeatEXEPath=dist/collectors/winlogbeat/windows/x86_64/winlogbeat.exe \
		-D BrandVendorName="$(BRAND_VENDOR_NAME)" \
		-D BrandVendorDisplay="$(BRAND_VENDOR_DISPLAY)" \
		-D BrandProductDisplay="$(BRAND_PRODUCT_DISPLAY)" \
		-D BrandProductLower="$(BRAND_PRODUCT_LOWER)" \
		-D BrandProductNameLower="$(BRAND_PRODUCT_NAME_LOWER)" \
		-D BrandDocsUrl="$(BRAND_DOCS_URL)" \
		-D BrandWinVendorDir="$(BRAND_WIN_VENDOR_DIR)" \
		-D BrandWinProductDir="$(BRAND_WIN_PRODUCT_DIR)" \
		-o dist/pkg/$(BRAND_PRODUCT_LOWER)-$(WINDOWS_INSTALLER_VERSION).msi \
		dist/msi-package.wxs

.PHONY: sign-windows-installer
sign-windows-installer:
	# This needs to run in a Docker container with the graylog/internal-codesigntool image
	codesigntool sign dist/pkg/$(BRAND_PRODUCT_LOWER)_installer_$(WINDOWS_INSTALLER_VERSION).exe
	codesigntool sign dist/pkg/$(BRAND_PRODUCT_LOWER)-$(WINDOWS_INSTALLER_VERSION).msi

.PHONY: package-chocolatey
package-chocolatey: ## Create Chocolatey .nupkg file
	# This needs to run in a Docker container based on the Dockerfile.chocolatey image!
	BRAND_PRODUCT_LOWER="$(BRAND_PRODUCT_LOWER)" \
	BRAND_PRODUCT_DISPLAY="$(BRAND_PRODUCT_DISPLAY)" \
	BRAND_VENDOR_NAME="$(BRAND_VENDOR_NAME)" \
	BRAND_GITHUB_URL="$(BRAND_GITHUB_URL)" \
	BRAND_ICON_URL="$(BRAND_ICON_URL)" \
	BRAND_DOCS_URL="$(BRAND_DOCS_URL)" \
	BRAND_DOWNLOADS_URL="$(BRAND_DOWNLOADS_URL)" \
	BRAND_HOMEPAGE_URL="$(BRAND_HOMEPAGE_URL)" \
	BRAND_MAINTAINER="$(BRAND_MAINTAINER)" \
	dist/chocolatey/gensha.sh $(COLLECTOR_VERSION)$(COLLECTOR_VERSION_SUFFIX) $(WINDOWS_INSTALLER_VERSION)
	# The fourth number in Chocolatey (NuGet) is the revision.
	# See: https://learn.microsoft.com/en-us/nuget/concepts/package-versioning#where-nugetversion-diverges-from-semantic-versioning
	cd dist/chocolatey && choco pack $(BRAND_PRODUCT_LOWER).nuspec --version $(CHOCOLATEY_VERSION) --out ../pkg

.PHONY: push-chocolatey
push-chocolatey: ## Push Chocolatey .nupkg file
	# This needs to run in a Docker container based on the Dockerfile.chocolatey image!
	# Escape the CHOCO_API_KEY to avoid printing it in the logs!
	choco push dist/pkg/$(BRAND_PRODUCT_LOWER).$(CHOCOLATEY_VERSION).nupkg -k=$$CHOCO_API_KEY

.PHONY: package-tar
package-tar: ## Create tar archive for all platforms
	@mkdir -p dist/pkg
	@$(TAR) --transform="s|/build|/$(BRAND_PRODUCT_LOWER)|" --transform="s|/dist|/$(BRAND_PRODUCT_LOWER)|" \
		-Pczf dist/pkg/$(BRAND_PRODUCT_LOWER)-$(COLLECTOR_VERSION)$(COLLECTOR_VERSION_SUFFIX).tar.gz \
		./build \
		./dist/collectors/auditbeat/linux/arm64/auditbeat \
		./dist/collectors/auditbeat/linux/x86_64/auditbeat \
		./dist/collectors/filebeat/linux/arm64/filebeat \
		./dist/collectors/filebeat/linux/x86_64/filebeat \
		./sidecar-example.yml \
		./sidecar-windows-example.yml

.PHONY: help
help:
	@grep -hE '^[a-zA-Z0-9_-]+:.*?## .*$$' $(MAKEFILE_LIST) | sort | $(AWK) 'BEGIN {FS = ":.*?## "}; {printf "\033[36m%-30s\033[0m %s\n", $$1, $$2}'

.DEFAULT_GOAL := all
