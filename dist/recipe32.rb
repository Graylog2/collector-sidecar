require_relative 'tools'

class GraylogSidecar < FPM::Cookery::Recipe
  description 'Graylog sidecar'

  name     'graylog-sidecar'
  version  data.version
  revision data.revision
  homepage 'https://graylog.org'
  arch     'i386'

  source   "file:../../build/#{version}/linux/386/graylog-sidecar"

  maintainer 'Graylog, Inc. <hello@graylog.org>'
  vendor     'graylog'
  license    'GPLv3'

  config_files '/etc/graylog/sidecar/sidecar.yml'

  fpm_attributes rpm_os: 'linux'

  def build
  end

  def install
    bin.install 'graylog-sidecar'
    usr('lib/graylog-sidecar').install '../../collectors/filebeat/linux/x86/filebeat'
    etc('graylog/sidecar').install '../../../sidecar-example.yml', 'sidecar.yml'
    etc('graylog/sidecar/generated').mkdir
    var('log/graylog/sidecar').mkdir
    var('run/graylog/sidecar').mkdir
    var('spool/graylog-sidecar/nxlog').mkdir
  end
end
