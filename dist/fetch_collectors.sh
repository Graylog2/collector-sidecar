#!/bin/bash
# Copyright (C)  2026 Graylog, Inc.
#
# This program is free software: you can redistribute it and/or modify
# it under the terms of the Server Side Public License, version 1,
# as published by MongoDB, Inc.
#
# This program is distributed in the hope that it will be useful,
# but WITHOUT ANY WARRANTY; without even the implied warranty of
# MERCHANTABILITY or FITNESS FOR A PARTICULAR PURPOSE. See the
# Server Side Public License for more details.
#
# You should have received a copy of the Server Side Public License
# along with this program. If not, see
# <http://www.mongodb.com/licensing/server-side-public-license>.
#
# SPDX-License-Identifier: SSPL-1.0


set -eo pipefail

FILEBEAT_VERSION=8.9.0
FILEBEAT_VERSION_32=7.17.12
WINLOGBEAT_VERSION=8.9.0
WINLOGBEAT_VERSION_32=7.17.12
AUDITBEAT_VERSION=8.9.0
AUDITBEAT_VERSION_32=7.17.12

# $1: beat name
# $2: beat operating system
# $3: beat version
# $4: beat architecture
download_beat()
{
  local name="$1"
  local os="$2"
  local version="$3"
  local arch="$4"

  mkdir -p dist/collectors/${name}/${os}/${arch}
  case "${os}" in
  "windows")
    archive="/tmp/${name}-${version}-${os}-${arch}.zip"
    if [ ! -f $archive ]; then
      echo "==> Downloading ${name}-${version}-${os}-${arch}"
      curl -fsSL -o $archive https://artifacts.elastic.co/downloads/beats/${name}/${name}-oss-${version}-${os}-${arch}.zip
    fi
    unzip -o -d dist/collectors/${name}/${os}/${arch} $archive
    mv dist/collectors/${name}/${os}/${arch}/${name}-${version}-${os}-${arch}/* dist/collectors/${name}/${os}/${arch}/
    rm -r dist/collectors/${name}/${os}/${arch}/${name}-${version}-${os}-${arch}
    ;;
  "linux")
    archive="/tmp/${name}-${version}-${os}-${arch}.tar.gz"
    if [ ! -f $archive ]; then
      echo "==> Downloading ${name}-${version}-${os}-${arch}"
      curl -fsSL -o $archive https://artifacts.elastic.co/downloads/beats/${name}/${name}-oss-${version}-${os}-${arch}.tar.gz
    fi
    tar -xzf $archive --strip-components=1 -C dist/collectors/${name}/${os}/${arch}
    ;;
  esac
}

download_beat "filebeat" "linux" ${FILEBEAT_VERSION} x86_64
download_beat "filebeat" "linux" ${FILEBEAT_VERSION_32} x86
download_beat "filebeat" "linux" ${FILEBEAT_VERSION} arm64

download_beat "auditbeat" "linux" ${AUDITBEAT_VERSION} x86_64
download_beat "auditbeat" "linux" ${AUDITBEAT_VERSION_32} x86
download_beat "auditbeat" "linux" ${AUDITBEAT_VERSION} arm64

download_beat "filebeat" "windows" ${FILEBEAT_VERSION} x86_64
download_beat "filebeat" "windows" ${FILEBEAT_VERSION_32} x86

download_beat "winlogbeat" "windows" ${WINLOGBEAT_VERSION} x86_64
download_beat "winlogbeat" "windows" ${WINLOGBEAT_VERSION_32} x86

