#!/usr/bin/env bash
# gensha.sh - Generate sha256 file for Chocolatey package and update chocolateyinstall.ps1 with the correct version and checksum. 

COLLECTOR_VERSION=$1
COLLECTOR_REVISION=$2
COLLECTOR_VERSION_SUFFIX=$3

if [[ ${COLLECTOR_VERSION_SUFFIX} == "-SNAPSHOT" ]]; then
  COLLECTOR_CHECKSUM=$(sha256sum dist/pkg/graylog_sidecar_installer_${COLLECTOR_VERSION}-${COLLECTOR_REVISION}.SNAPSHOT.exe | cut -d" " -f1)
else
  COLLECTOR_CHECKSUM=$(sha256sum dist/pkg/graylog_sidecar_installer_${COLLECTOR_VERSION}-${COLLECTOR_REVISION}.exe | cut -d" " -f1)
fi

root_url="https://downloads.graylog.org/releases/graylog-collector-sidecar"
version_url="${root_url}/${COLLECTOR_VERSION}/graylog_sidecar_installer_${COLLECTOR_VERSION}-${COLLECTOR_REVISION}.exe"

sed -e "s,%%CHECKSUM%%,$COLLECTOR_CHECKSUM,g" \
	-e "s,%%URL%%,$version_url,g" \
	"dist/chocolatey/tools/chocolateyinstall.ps1.template" \
	> "dist/chocolatey/tools/chocolateyinstall.ps1"
