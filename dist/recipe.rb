class GraylogSidecar < FPM::Cookery::Recipe
  description 'Graylog sidecar'

  name     'sidecar'
  version  '0.0.1'
  revision '1'
  homepage 'https://graylog.org'
  arch     'amd64'

  source   'file:../../sidecar'

  maintainer 'Graylog, Inc. <hello@graylog.org>'
  vendor     'graylog'
  license    'GPLv3'

  config_files '/etc/graylog/sidecar/sidecar.ini'

  def build
  end

  def install
    bin.install 'sidecar'
    etc('graylog/sidecar').install '../../../sidecar.ini'
    etc('graylog/sidecar/generated').mkdir
    var('log/graylog/sidecar').mkdir
    var('run/graylog/sidecar').mkdir
  end
end
