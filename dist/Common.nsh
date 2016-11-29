;---------------------------------------------------  
; Shared Functions between install and uninstall

!macro CheckAdmin un
Function ${un}CheckAdmin
  UserInfo::GetAccountType
  pop $0
  ${If} $0 != "admin" ;Require admin rights on NT4+
    MessageBox MB_OK|MB_ICONEXCLAMATION "Administrator rights required!"  /SD IDOK
    Abort
  ${EndIf}
FunctionEnd
!macroend
!insertmacro CheckAdmin ""
!insertmacro CheckAdmin "un."

!macro CheckConcurrent un
Function ${un}CheckConcurrent
  ;Prevent Multiple Instances of the installer
  System::Call 'kernel32::CreateMutexA(i 0, i 0, t "${MUI_BRANDINGTEXT}") i .r1 ?e'
  Pop $R0
  StrCmp $R0 0 +3
    MessageBox MB_OK|MB_ICONEXCLAMATION "The un/installer is already running."  /SD IDOK
    Abort
FunctionEnd
!macroend
!insertmacro CheckConcurrent ""
!insertmacro CheckConcurrent "un."
