class GraylogSidecar < FPM::Cookery::Recipe
  description 'Graylog sidecar'

  name     'sidecar'
  version  '1.0.0'
  revision '1'
  homepage 'https://graylog.org'
  arch     'amd64'

  source   'file:../../sidecar'

  maintainer 'Graylog, Inc. <hello@graylog.org>'
  vendor     'graylog'
  license    'GPLv3'

  config_files '/etc/sidecar/sidecar.ini'

  def build
  end

  def install
    bin.install 'sidecar'
    etc('sidecar').install '../../../sidecar.ini'
    etc('sidecar/generated').mkdir
    var('log/sidecar').mkdir
  end
end
