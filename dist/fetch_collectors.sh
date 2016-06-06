#!/bin/bash

ARCHS=( i686 x86_64 windows )
FILEBEAT_VERSION=1.2.3
WINLOGBEAT_VERSION=1.2.3

# $1: beat name
# $2: beat version
# $3: beat arch
download_beat()
{
  mkdir -p dist/collectors/${1}/${3}
  if [ "${3}" == "windows" ]
    then
      curl -o /tmp/${1}.zip https://download.elastic.co/beats/${1}/${1}-${2}-${3}.zip
      unzip -o -d dist/collectors/${1}/${3} /tmp/${1}.zip
      mv dist/collectors/${1}/${3}/${1}-${2}-windows/* dist/collectors/${1}/${3}/
      rm -r /tmp/${1}.zip dist/collectors/${1}/${3}/${1}-${2}-windows
    else
      curl https://download.elastic.co/beats/${1}/${1}-${2}-${3}.tar.gz | tar -xz --strip-components=1 -C dist/collectors/${1}/${3}
  fi
}

for ARCH in "${ARCHS[@]}"
do
  download_beat "filebeat" ${FILEBEAT_VERSION} ${ARCH}
done

download_beat "winlogbeat" ${WINLOGBEAT_VERSION} ${ARCH}
