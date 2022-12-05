#!/usr/bin/env bash
# gensha.sh - Generate sha256 file for Chocolatey package and update chocolateyinstall.ps1 with the correct version and checksum. 

set -eo pipefail

COLLECTOR_VERSION="$1"
COLLECTOR_INSTALLER_VERSION="$2"

COLLECTOR_CHECKSUM="$(sha256sum dist/pkg/graylog_sidecar_installer_${COLLECTOR_INSTALLER_VERSION}.exe | cut -d" " -f1)"

root_url="https://downloads.graylog.org/releases/graylog-collector-sidecar"
version_url="${root_url}/${COLLECTOR_VERSION}/graylog_sidecar_installer_${COLLECTOR_INSTALLER_VERSION}.exe"

sed -e "s,%%CHECKSUM%%,$COLLECTOR_CHECKSUM,g" \
	-e "s,%%URL%%,$version_url,g" \
	"dist/chocolatey/tools/chocolateyinstall.ps1.template" \
	> "dist/chocolatey/tools/chocolateyinstall.ps1"
