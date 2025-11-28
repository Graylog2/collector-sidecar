# Branding configuration for FPM recipes
# Reads from environment variables (set by Makefile)
module Branding
  def self.vendor_name
    ENV.fetch('BRAND_VENDOR_NAME', 'Graylog')
  end

  def self.vendor_display
    ENV.fetch('BRAND_VENDOR_DISPLAY', 'Graylog, Inc.')
  end

  def self.vendor_lower
    ENV.fetch('BRAND_VENDOR_LOWER', 'graylog')
  end

  def self.product_name
    ENV.fetch('BRAND_PRODUCT_NAME', 'Sidecar')
  end

  def self.product_name_lower
    ENV.fetch('BRAND_PRODUCT_NAME_LOWER', 'sidecar')
  end

  def self.product_lower
    ENV.fetch('BRAND_PRODUCT_LOWER', 'graylog-sidecar')
  end

  def self.homepage_url
    ENV.fetch('BRAND_HOMEPAGE_URL', 'https://graylog.org')
  end

  def self.maintainer
    ENV.fetch('BRAND_MAINTAINER', 'Graylog, Inc. <hello@graylog.org>')
  end

  def self.license
    ENV.fetch('BRAND_LICENSE', 'SSPL')
  end

  def self.config_dir
    ENV.fetch('BRAND_CONFIG_DIR_UNIX', '/etc/graylog/sidecar')
  end

  def self.lib_dir
    ENV.fetch('BRAND_LIB_DIR_UNIX', '/usr/lib/graylog-sidecar')
  end

  def self.log_dir
    ENV.fetch('BRAND_LOG_DIR_UNIX', '/var/log/graylog-sidecar')
  end

  def self.cache_dir
    ENV.fetch('BRAND_CACHE_DIR_UNIX', '/var/cache/graylog-sidecar')
  end

  def self.var_lib_dir
    ENV.fetch('BRAND_VAR_LIB_DIR_UNIX', '/var/lib/graylog-sidecar')
  end

  def self.var_run_dir
    ENV.fetch('BRAND_VAR_RUN_DIR_UNIX', '/var/run/graylog-sidecar')
  end

  # Convenience method for etc path based on vendor/product
  def self.etc_path
    "#{vendor_lower}/#{product_name_lower}"
  end
end
