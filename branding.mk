# Branding configuration for Graylog Sidecar
# Override these variables via environment or by replacing this file

# Core branding names
BRAND_VENDOR_NAME ?= Graylog
BRAND_VENDOR_DISPLAY ?= Graylog, Inc.
BRAND_VENDOR_EMAIL ?= hello@graylog.org
BRAND_VENDOR_DOMAIN ?= graylog.org
BRAND_PRODUCT_NAME ?= Sidecar

# Derived names (computed from core names)
BRAND_PRODUCT_DISPLAY ?= $(BRAND_VENDOR_NAME) $(BRAND_PRODUCT_NAME)
BRAND_PRODUCT_LOWER ?= $(shell echo "$(BRAND_VENDOR_NAME)-$(BRAND_PRODUCT_NAME)" | tr '[:upper:]' '[:lower:]')

# URLs
BRAND_HOMEPAGE_URL ?= https://www.graylog.org
BRAND_DOCS_URL ?= https://docs.graylog.org/
BRAND_DOWNLOADS_URL ?= https://downloads.graylog.org/releases/graylog-collector-sidecar
BRAND_GITHUB_URL ?= https://github.com/Graylog2/collector-sidecar
BRAND_ICON_URL ?= https://rawcdn.githack.com/Graylog2/collector-sidecar/c32a05ba052815ebbdeb8588395451dd5b2c1378/images/graylog-icon.png

# File names
BRAND_ICON_FILE ?= dist/graylog.ico
BRAND_BINARY_NAME ?= $(BRAND_PRODUCT_LOWER)

# Paths (Unix)
BRAND_VENDOR_LOWER ?= $(shell echo "$(BRAND_VENDOR_NAME)" | tr '[:upper:]' '[:lower:]')
BRAND_PRODUCT_NAME_LOWER ?= $(shell echo "$(BRAND_PRODUCT_NAME)" | tr '[:upper:]' '[:lower:]')
BRAND_CONFIG_DIR_UNIX ?= /etc/$(BRAND_VENDOR_LOWER)/$(BRAND_PRODUCT_NAME_LOWER)
BRAND_LIB_DIR_UNIX ?= /usr/lib/$(BRAND_PRODUCT_LOWER)
BRAND_LOG_DIR_UNIX ?= /var/log/$(BRAND_PRODUCT_LOWER)
BRAND_CACHE_DIR_UNIX ?= /var/cache/$(BRAND_PRODUCT_LOWER)
BRAND_VAR_LIB_DIR_UNIX ?= /var/lib/$(BRAND_PRODUCT_LOWER)
BRAND_VAR_RUN_DIR_UNIX ?= /var/run/$(BRAND_PRODUCT_LOWER)
BRAND_ICON_FILE_ABS ?= $(realpath $(BRAND_ICON_FILE))

# Paths (Windows) - use consistent naming
BRAND_WIN_VENDOR_DIR ?= $(BRAND_VENDOR_NAME)
BRAND_WIN_PRODUCT_DIR ?= $(BRAND_PRODUCT_NAME_LOWER)

# Package metadata
BRAND_LICENSE ?= SSPL
BRAND_MAINTAINER ?= $(BRAND_VENDOR_DISPLAY) <$(BRAND_VENDOR_EMAIL)>

# Windows registry key (no spaces, CamelCase)
BRAND_REGISTRY_KEY ?= $(BRAND_VENDOR_NAME)$(BRAND_PRODUCT_NAME)
