#!/usr/bin/env bash
# gensha.sh - Generate sha256 file for Chocolatey package and update chocolateyinstall.ps1 with the correct version and checksum.

set -eo pipefail

COLLECTOR_VERSION="$1"
COLLECTOR_INSTALLER_VERSION="$2"

# Branding defaults (can be overridden via environment)
BRAND_PRODUCT_LOWER="${BRAND_PRODUCT_LOWER:-graylog-sidecar}"
BRAND_PRODUCT_DISPLAY="${BRAND_PRODUCT_DISPLAY:-Graylog Sidecar}"
BRAND_VENDOR_NAME="${BRAND_VENDOR_NAME:-Graylog}"
BRAND_GITHUB_URL="${BRAND_GITHUB_URL:-https://github.com/Graylog2/collector-sidecar}"
BRAND_ICON_URL="${BRAND_ICON_URL:-https://rawcdn.githack.com/Graylog2/collector-sidecar/c32a05ba052815ebbdeb8588395451dd5b2c1378/images/graylog-icon.png}"
BRAND_DOCS_URL="${BRAND_DOCS_URL:-https://docs.graylog.org/docs/sidecar}"
BRAND_DOWNLOADS_URL="${BRAND_DOWNLOADS_URL:-https://downloads.graylog.org/releases/graylog-collector-sidecar}"

COLLECTOR_CHECKSUM="$(sha256sum dist/pkg/${BRAND_PRODUCT_LOWER}_installer_${COLLECTOR_INSTALLER_VERSION}.exe | cut -d" " -f1)"

version_url="${BRAND_DOWNLOADS_URL}/${COLLECTOR_VERSION}/${BRAND_PRODUCT_LOWER}_installer_${COLLECTOR_INSTALLER_VERSION}.exe"

# Generate chocolateyinstall.ps1 from template
sed -e "s,%%CHECKSUM%%,$COLLECTOR_CHECKSUM,g" \
	-e "s,%%URL%%,$version_url,g" \
	-e "s,%%BRAND_PRODUCT_LOWER%%,${BRAND_PRODUCT_LOWER},g" \
	"dist/chocolatey/tools/chocolateyinstall.ps1.template" \
	> "dist/chocolatey/tools/chocolateyinstall.ps1"

# Generate nuspec from template
sed -e "s|%%BRAND_PRODUCT_LOWER%%|${BRAND_PRODUCT_LOWER}|g" \
	-e "s|%%BRAND_PRODUCT_DISPLAY%%|${BRAND_PRODUCT_DISPLAY}|g" \
	-e "s|%%BRAND_VENDOR_NAME%%|${BRAND_VENDOR_NAME}|g" \
	-e "s|%%BRAND_GITHUB_URL%%|${BRAND_GITHUB_URL}|g" \
	-e "s|%%BRAND_ICON_URL%%|${BRAND_ICON_URL}|g" \
	-e "s|%%BRAND_DOCS_URL%%|${BRAND_DOCS_URL}|g" \
	"dist/chocolatey/sidecar.nuspec.template" \
	> "dist/chocolatey/${BRAND_PRODUCT_LOWER}.nuspec"
