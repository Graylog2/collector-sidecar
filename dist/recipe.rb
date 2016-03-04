class GraylogSidecar < FPM::Cookery::Recipe
  description 'Graylog collector sidecar'

  name     'collector-sidecar'
  version  '0.0.1'
  revision '1'
  homepage 'https://graylog.org'
  arch     'amd64'

  source   'file:../../graylog-collector-sidecar'

  maintainer 'Graylog, Inc. <hello@graylog.org>'
  vendor     'graylog'
  license    'Apache2'

  config_files '/etc/graylog/collector-sidecar/collector_sidecar.ini'

  def build
  end

  def install
    bin.install 'graylog-collector-sidecar'
    etc('graylog/collector-sidecar').install '../../../collector_sidecar.ini'
    etc('graylog/collector-sidecar/generated').mkdir
    var('log/graylog/collector-sidecar').mkdir
    var('run/graylog/collector-sidecar').mkdir
  end
end
