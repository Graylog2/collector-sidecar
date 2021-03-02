#!/usr/bin/env bash

COLLECTOR_VERSION=$1
COLLECTOR_REVISION=$2
COLLECTOR_VERSION_SUFFIX=$3

if [[ ${COLLECTOR_VERSION_SUFFIX} == "-SNAPSHOT" ]]; then
  COLLECTOR_CHECKSUM=$(sha256sum dist/pkg/graylog_sidecar_installer_${COLLECTOR_VERSION}-${COLLECTOR_REVISION}.SNAPSHOT.exe | cut -d" " -f1)
else
  COLLECTOR_CHECKSUM=$(sha256sum dist/pkg/graylog_sidecar_installer_${COLLECTOR_VERSION}-${COLLECTOR_REVISION}.exe | cut -d" " -f1)
fi

sed -i "s/checksum      = '.*'/checksum      = '$COLLECTOR_CHECKSUM'/" dist/chocolatey/tools/chocolateyinstall.ps1
sed -i "s/url           = '.*'/url           = 'https:\/\/downloads.graylog.org\/releases\/graylog-collector-sidecar\/${COLLECTOR_VERSION}\/graylog_sidecar_installer_${COLLECTOR_VERSION}-${COLLECTOR_REVISION}.exe'/" dist/chocolatey/tools/chocolateyinstall.ps1

find dist/pkg -name "graylog_sidecar_installer*.exe" -exec /bin/bash -c "sha256sum {} | cut -d' ' -f1 > {}.sha256.txt" \;
