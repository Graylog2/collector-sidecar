!macro _GetFullComputerName
  Push $R0
  Push $R1

  ReadRegStr $0 HKLM "System\CurrentControlSet\Control\ComputerName\ActiveComputerName" "ComputerName"
  StrCpy $R0 $0

  StrCpy $0 0
  EnumRegKey $1 HKLM "System\CurrentControlSet\Services\Tcpip\Parameters\DNSRegisteredAdapters" $0
  ReadRegStr $0 HKLM "System\CurrentControlSet\Services\Tcpip\Parameters\DNSRegisteredAdapters\$1" "PrimaryDomainName"

  StrCpy $R1 "$R0.$0"

  Push "$R1"

!macroend
