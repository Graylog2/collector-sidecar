#!/bin/bash

ARCHS=( i686 x86_64 windows )
FILEBEAT_VERSION=1.2.3
WINLOGBEAT_VERSION=1.2.3

# $1: beat name
# $2: beat version
# $3: beat arch
download_beat()
{
  local name="$1"
  local version="$2"
  local arch="$3"

  mkdir -p dist/collectors/${name}/${arch}
  if [ "${arch}" == "windows" ]
    then
      archive="/tmp/${name}-${version}.zip"
      if [ ! -f $archive ]; then
        echo "==> Downloading ${name}-${version}-${arch}"
        curl -o $archive https://download.elastic.co/beats/${name}/${name}-${version}-${arch}.zip
      fi
      unzip -o -d dist/collectors/${name}/${arch} $archive
      mv dist/collectors/${name}/${arch}/${name}-${version}-windows/* dist/collectors/${name}/${arch}/
      rm -r dist/collectors/${name}/${arch}/${name}-${version}-windows
    else
      archive="/tmp/${name}-${version}.tar.gz"
      if [ ! -f $archive ]; then
        echo "==> Downloading ${name}-${version}-${arch}"
        curl -o $archive https://download.elastic.co/beats/${name}/${name}-${version}-${arch}.tar.gz
      fi
      tar -xzf $archive --strip-components=1 -C dist/collectors/${name}/${arch}
  fi
}

for ARCH in "${ARCHS[@]}"
do
  download_beat "filebeat" ${FILEBEAT_VERSION} ${ARCH}
done

download_beat "winlogbeat" ${WINLOGBEAT_VERSION} ${ARCH}
