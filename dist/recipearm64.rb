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

require_relative 'tools'

class GraylogSidecar < FPM::Cookery::Recipe
  description 'Graylog collector sidecar'

  name     'graylog-sidecar'
  version  data.version
  revision data.revision
  homepage 'https://graylog.org'
  arch     'arm64'

  source   "file:../../build/#{version}/linux/arm64/graylog-sidecar"

  maintainer 'Graylog, Inc. <packages@graylog.com>'
  vendor     'graylog'
  license    'SSPL'

  config_files '/etc/graylog/sidecar/sidecar.yml'

  fpm_attributes rpm_os: 'linux'

  def build
  end

  def install
    bin.install 'graylog-sidecar'
    lib('graylog-sidecar').install '../../collectors/filebeat/linux/arm64/filebeat'
    lib('graylog-sidecar').install '../../collectors/auditbeat/linux/arm64/auditbeat'
    etc('graylog/sidecar').install '../../../sidecar-example.yml', 'sidecar.yml'
    etc('graylog/sidecar/sidecar.yml').chmod(0600)
    var('lib/graylog-sidecar/generated').mkdir
    var('log/graylog-sidecar').mkdir
    var('run/graylog-sidecar').mkdir
  end
end
