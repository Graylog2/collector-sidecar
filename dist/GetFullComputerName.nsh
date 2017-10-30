!macro _GetFullComputerName

  Var /GLOBAL ComputerName
  Var /GLOBAL PrimaryDomainName
  Var /GLOBAL FullComputerName

  ReadRegStr $0 HKLM "System\CurrentControlSet\Control\ComputerName\ActiveComputerName" "ComputerName"
  StrCpy $ComputerName $0

  StrCpy $0 0
  EnumRegKey $1 HKLM "System\CurrentControlSet\Services\Tcpip\Parameters\DNSRegisteredAdapters" $0
  ReadRegStr $0 HKLM "System\CurrentControlSet\Services\Tcpip\Parameters\DNSRegisteredAdapters\$1" "PrimaryDomainName"

  StrCpy $PrimaryDomainName $0
  StrCpy $FullComputerName "$ComputerName.$PrimaryDomainName"

  Push $FullComputerName

!macroend
