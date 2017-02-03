require_relative 'tools'

class GraylogSidecar < FPM::Cookery::Recipe
  description 'Graylog collector sidecar'

  name     'collector-sidecar'
  version  data.version
  revision data.revision
  homepage 'https://graylog.org'
  arch     'i386'

  source   "file:../../build/#{version}/linux/386/graylog-collector-sidecar"

  maintainer 'Graylog, Inc. <hello@graylog.org>'
  vendor     'graylog'
  license    'GPLv3'

  config_files '/etc/graylog/collector-sidecar/collector_sidecar.yml'

  fpm_attributes rpm_os: 'linux'

  def build
  end

  def install
    bin.install 'graylog-collector-sidecar'
    bin.install '../../collectors/filebeat/linux/x86/filebeat'
    etc('graylog/collector-sidecar').install '../../../collector_sidecar.yml'
    etc('graylog/collector-sidecar/generated').mkdir
    var('log/graylog/collector-sidecar').mkdir
    var('run/graylog/collector-sidecar').mkdir
    var('spool/collector-sidecar/nxlog').mkdir
  end
end
