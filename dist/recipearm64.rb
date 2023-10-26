require_relative 'tools'

class GraylogSidecar < FPM::Cookery::Recipe
  description 'Graylog collector sidecar'

  name     'graylog-sidecar'
  version  data.version
  revision data.revision
  homepage 'https://graylog.org'
  arch     'arm64'

  source   "file:../../build/#{version}/linux/arm64/graylog-sidecar"

  maintainer 'Graylog, Inc. <hello@graylog.org>'
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
    var('lib/graylog-sidecar/generated').mkdir
    var('log/graylog-sidecar').mkdir
    var('run/graylog-sidecar').mkdir
  end
end
