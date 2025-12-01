require_relative 'tools'
require_relative 'branding'

class GraylogSidecar < FPM::Cookery::Recipe
  description "#{Branding.vendor_name} collector sidecar"

  name     Branding.product_lower
  version  data.version
  revision data.revision
  homepage Branding.homepage_url
  arch     'arm64'

  source   "file:../../build/#{version}/linux/arm64/graylog-sidecar"

  maintainer Branding.maintainer
  vendor     Branding.vendor_lower
  license    Branding.license

  config_files "#{Branding.config_dir}/sidecar.yml"

  fpm_attributes rpm_os: 'linux'

  def build
  end

  def install
    bin.install 'graylog-sidecar', Branding.product_lower
    lib(Branding.product_lower).install '../../collectors/filebeat/linux/arm64/filebeat'
    lib(Branding.product_lower).install '../../collectors/auditbeat/linux/arm64/auditbeat'
    etc(Branding.etc_path).install '../../../sidecar-example.yml', 'sidecar.yml'
    etc("#{Branding.etc_path}/sidecar.yml").chmod(0600)
    var("lib/#{Branding.product_lower}/generated").mkdir
    var("log/#{Branding.product_lower}").mkdir
    var("run/#{Branding.product_lower}").mkdir
  end
end
