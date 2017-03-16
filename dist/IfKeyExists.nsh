!macro _IfKeyExists ROOT MAIN_KEY KEY
  Push $R0
  Push $R1
 
  !define Index 'Line${__LINE__}'
   
  StrCpy $R1 "0"
   
  "${Index}-Loop:"
  ; Check for Key
  EnumRegKey $R0 ${ROOT} "${MAIN_KEY}" "$R1"
  StrCmp $R0 "" "${Index}-False"
  IntOp $R1 $R1 + 1
  StrCmp $R0 "${KEY}" "${Index}-True" "${Index}-Loop"
   
  "${Index}-True:"
  ;Return 1 if found
  Push "1"
  Goto "${Index}-End"
   
  "${Index}-False:"
  ;Return 0 if not found
  Push "0"
  Goto "${Index}-End"
   
  "${Index}-End:"
  !undef Index
  Exch 2
  Pop $R0
  Pop $R1
!macroend
