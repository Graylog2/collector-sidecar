$ErrorActionPreference = 'Stop';
$toolsDir   = "$(Split-Path -parent $MyInvocation.MyCommand.Definition)"

$packageArgs = @{
  packageName   = $env:ChocolateyPackageName
  unzipLocation = $toolsDir
  fileType      = 'EXE'
  url           = 'https://downloads.graylog.org/releases/graylog-collector-sidecar/1.1.0/graylog_sidecar_installer_1.1.0-1.exe'
  softwareName  = 'graylog-sidecar*'
  checksum      = '55693ad815021985d8ecc733e918e285dc892233035b6dcac9c11b480bf1af42'
  checksumType  = 'sha256'
  silentArgs   = '/S'
}

Install-ChocolateyPackage @packageArgs
