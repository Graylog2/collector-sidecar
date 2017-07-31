#!/bin/bash

ARCHS=( x86 x86_64 )
FILEBEAT_VERSION=5.5.1
WINLOGBEAT_VERSION=5.5.1

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
      curl -o $archive https://artifacts.elastic.co/downloads/beats/${name}/${name}-${version}-${os}-${arch}.zip
    fi
    unzip -o -d dist/collectors/${name}/${os}/${arch} $archive
    mv dist/collectors/${name}/${os}/${arch}/${name}-${version}-${os}-${arch}/* dist/collectors/${name}/${os}/${arch}/
    rm -r dist/collectors/${name}/${os}/${arch}/${name}-${version}-${os}-${arch}
    ;;
  "linux")
    archive="/tmp/${name}-${version}-${os}-${arch}.tar.gz"
    if [ ! -f $archive ]; then
      echo "==> Downloading ${name}-${version}-${os}-${arch}"
      curl -o $archive https://artifacts.elastic.co/downloads/beats/${name}/${name}-${version}-${os}-${arch}.tar.gz
    fi
    tar -xzf $archive --strip-components=1 -C dist/collectors/${name}/${os}/${arch}
    ;;
  esac
}

for ARCH in "${ARCHS[@]}"
do
  download_beat "filebeat" "linux" ${FILEBEAT_VERSION} ${ARCH}
  download_beat "filebeat" "windows" ${FILEBEAT_VERSION} ${ARCH}
  download_beat "winlogbeat" "windows" ${WINLOGBEAT_VERSION} ${ARCH}
done

