$ErrorActionPreference = 'Stop';
$toolsDir   = "$(Split-Path -parent $MyInvocation.MyCommand.Definition)"

$packageArgs = @{
  packageName   = $env:ChocolateyPackageName
  unzipLocation = $toolsDir
  fileType      = 'EXE'
  url           = 'https://downloads.graylog.org/releases/graylog-collector-sidecar/1.0.2/graylog_sidecar_installer_1.0.2-1.exe'
  url64bit      = 'https://downloads.graylog.org/releases/graylog-collector-sidecar/1.0.2/graylog_sidecar_installer_1.0.2-1.exe'
  softwareName  = 'graylog-sidecar*'

  # Checksums are now required as of 0.10.0.
  # To determine checksums, you can get that from the original site if provided.
  # You can also use checksum.exe (choco install checksum) and use it
  # e.g. checksum -t sha256 -f path\to\file
  checksum      = ''
  checksumType  = 'sha256' #default is md5, can also be sha1, sha256 or sha512
  checksum64    = ''
  checksumType64= 'sha256' #default is checksumType
  silentArgs   = '/S'
}

Install-ChocolateyPackage @packageArgs
