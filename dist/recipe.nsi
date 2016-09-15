; -------------------------------
; Start
 
  Name "Graylog Collector Sidecar"
  !define VERSION "0.1.0"
  !define MUI_FILE "savefile"
  !define MUI_BRANDINGTEXT "Graylog Collector Sidecar v${VERSION}"
  CRCCheck On
 
  !include "${NSISDIR}\Contrib\Modern UI\System.nsh"
  !include nsDialogs.nsh
  !include LogicLib.nsh
  !include StrRep.nsh
  !include ReplaceInFile.nsh
  !include FileFunc.nsh
 
	VIProductVersion "0.${VERSION}"
	VIAddVersionKey "FileVersion" "${VERSION}"
  VIAddVersionKey "FileDescription" "Graylog Collector Sidecar"
  VIAddVersionKey "LegalCopyright" "Graylog, Inc."
 
;---------------------------------
;General
 
  OutFile "pkg/collector_sidecar_installer_${VERSION}_x64.exe"
  ShowInstDetails "nevershow"
  ShowUninstDetails "nevershow"
  SetCompressor "bzip2"

  ; Variables
  Var Params
  Var ParamServerUrl
  Var InputServerUrl
  Var ServerUrl
  Var Dialog
  Var Label

  ;Pages
  ;Page directory
  Page custom nsDialogsPage nsDialogsPageLeave
  Page instfiles

;--------------------------------
;Folder selection page
 
  InstallDir "$PROGRAMFILES64\graylog\collector-sidecar"
 
;--------------------------------
;Modern UI Configuration
 
  !define MUI_WELCOMEPAGE  
  !define MUI_LICENSEPAGE
  !define MUI_DIRECTORYPAGE
  !define MUI_ABORTWARNING
  !define MUI_UNINSTALLER
  !define MUI_UNCONFIRMPAGE
  !define MUI_FINISHPAGE  
 
;--------------------------------
;Language
 
  !insertmacro MUI_LANGUAGE "English"
 
;--------------------------------
;Data
 
  LicenseData "../COPYING"

;-------------------------------- 
;Installer Sections     
Section "Install"
 
  ;Add files
  SetOutPath "$INSTDIR\generated"  
  SetOutPath "$INSTDIR"
 
  File "../build/${VERSION}/windows/amd64/graylog-collector-sidecar.exe"
  File "collectors/winlogbeat/windows/winlogbeat.exe"
  File "collectors/filebeat/windows/filebeat.exe"
  File /oname=collector_sidecar.yml "../collector_sidecar_windows.yml"
  File "../COPYING"

  WriteUninstaller "$INSTDIR\uninstall.exe"

SectionEnd
 
Section "Post"

  ; Update configuration
  ${GetParameters} $Params
  ${GetOptions} $Params "-SERVERURL="  $ParamServerUrl
  ${If} $ParamServerUrl != ""
    StrCpy $ServerUrl $ParamServerUrl
  ${EndIf}
  ${If} $ServerUrl == ""
    ; default for silent install
    StrCpy $ServerUrl "http://127.0.0.1:9000/api"
  ${EndIf}
  !insertmacro _ReplaceInFile "$INSTDIR\collector_sidecar.yml" "<SERVERURL>" $ServerUrl

SectionEnd
 
;--------------------------------    
;Uninstaller Section  
Section "Uninstall"

  ;Uninstall system service
  ExecWait '"$INSTDIR\graylog-collector-sidecar.exe" -service stop'
  ExecWait '"$INSTDIR\graylog-collector-sidecar.exe" -service uninstall'
 
  ;Delete Files
  RMDir /r "$INSTDIR\*.*"    
 
  ;Remove the installation directory
  RMDir "$INSTDIR"
  RMDir "$PROGRAMFILES64\graylog"
 
SectionEnd
 
 
;--------------------------------    
;Functions
Function .onInstSuccess
  MessageBox MB_OK "You have successfully installed Graylog Collector Sidecar." /SD IDOK
FunctionEnd
 
Function un.onUninstSuccess
  MessageBox MB_OK "You have successfully uninstalled Graylog Collector Sidecar." /SD IDOK
FunctionEnd

Function nsDialogsPage
  nsDialogs::Create 1018
  Pop $Dialog

  ${If} $Dialog == error
     Abort
  ${EndIf}

  ${NSD_CreateLabel} 0 0 100% 12u "Please enter the URL to your Graylog API:"
  Pop $Label
  ${NSD_CreateText} 50 40 75% 12u "http://127.0.0.1:9000/api"
  Pop $InputServerUrl

  nsDialogs::Show
FunctionEnd

Function nsDialogsPageLeave
  ${NSD_GetText} $InputServerUrl $ServerUrl

  ${If} $ServerUrl == ""
      MessageBox MB_OK "Please enter a valid address to your Graylog server!"
      Abort
  ${EndIf}
FunctionEnd
