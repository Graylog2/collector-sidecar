$ErrorActionPreference = 'Stop';
$toolsDir   = "$(Split-Path -parent $MyInvocation.MyCommand.Definition)"

$packageArgs = @{
  packageName   = $env:ChocolateyPackageName
  unzipLocation = $toolsDir
  fileType      = 'EXE'
  url           = 'https://downloads.graylog.org/releases/graylog-collector-sidecar/1.1.0/graylog_sidecar_installer_1.1.0-1.exe'
  softwareName  = 'graylog-sidecar*'
  checksum      = '08ee296fa6970fec2026d5956bdfcf9eb622dba92c8e5f811ddf3a765041b810'
  checksumType  = 'sha256'
  silentArgs   = '/S'
}

Install-ChocolateyPackage @packageArgs
