#!/usr/bin/env bash
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
