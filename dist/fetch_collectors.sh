#!/bin/bash

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
      curl -o $archive https://artifacts.elastic.co/downloads/beats/${name}/${name}-oss-${version}-${os}-${arch}.zip
    fi
    unzip -o -d dist/collectors/${name}/${os}/${arch} $archive
    mv dist/collectors/${name}/${os}/${arch}/${name}-${version}-${os}-${arch}/* dist/collectors/${name}/${os}/${arch}/
    rm -r dist/collectors/${name}/${os}/${arch}/${name}-${version}-${os}-${arch}
    ;;
  "linux")
    archive="/tmp/${name}-${version}-${os}-${arch}.tar.gz"
    if [ ! -f $archive ]; then
      echo "==> Downloading ${name}-${version}-${os}-${arch}"
      curl -o $archive https://artifacts.elastic.co/downloads/beats/${name}/${name}-oss-${version}-${os}-${arch}.tar.gz
    fi
    tar -xzf $archive --strip-components=1 -C dist/collectors/${name}/${os}/${arch}
    ;;
  esac
}

download_beat "filebeat" "linux" ${FILEBEAT_VERSION} x86_64
download_beat "filebeat" "linux" ${FILEBEAT_VERSION_32} x86
download_beat "filebeat" "linux" ${FILEBEAT_VERSION_32} arm64

download_beat "auditbeat" "linux" ${FILEBEAT_VERSION} x86_64
download_beat "auditbeat" "linux" ${AUDITBEAT_VERSION_32} x86
download_beat "auditbeat" "linux" ${AUDITBEAT_VERSION_32} arm64

download_beat "filebeat" "windows" ${FILEBEAT_VERSION} x86_64
download_beat "filebeat" "windows" ${FILEBEAT_VERSION_32} x86

download_beat "winlogbeat" "windows" ${WINLOGBEAT_VERSION} x86_64
download_beat "winlogbeat" "windows" ${WINLOGBEAT_VERSION_32} x86

