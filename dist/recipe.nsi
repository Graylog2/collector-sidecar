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
  !include WordFunc.nsh

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
  Var ParamTags
  Var InputTags
  Var Tags
  Var Dialog
  Var Label

  ;Pages
  ;Page directory
  Page custom nsDialogsPage nsDialogsPageLeave
  Page instfiles

;--------------------------------
;Folder selection page
 
  InstallDir "$PROGRAMFILES64\Graylog\collector-sidecar"
 
;--------------------------------
;Modern UI Configuration
 
  !define MUI_WELCOMEPAGE  
  !define MUI_LICENSEPAGE
  !define MUI_DIRECTORYPAGE
  !define MUI_ABORTWARNING
  !define MUI_UNINSTALLER
  !define MUI_UNCONFIRMPAGE
  !define MUI_FINISHPAGE
  !define MUI_ICON "graylog.ico"  
 
;--------------------------------
;Macros
 
  !insertmacro MUI_LANGUAGE "English"
  !insertmacro WordFind
  !insertmacro WordFind2X

;--------------------------------
;Data
 
  LicenseData "../COPYING"

;-------------------------------- 
;Installer Sections     
Section "Install"
 
  ;Add files
  SetOutPath "$INSTDIR"
 
  File "../build/${VERSION}/windows/amd64/graylog-collector-sidecar.exe"
  File "collectors/winlogbeat/windows/winlogbeat.exe"
  File "collectors/filebeat/windows/filebeat.exe"
  File /oname=collector_sidecar.yml "../collector_sidecar_windows.yml"
  File "../COPYING"
  File "graylog.ico"

  WriteUninstaller "$INSTDIR\uninstall.exe"
	
	
  WriteRegStr HKLM "Software\Microsoft\Windows\CurrentVersion\Uninstall\GraylogCollectorSidecar" \
                 "DisplayName" "Graylog Collector Sidecar"
  WriteRegStr HKLM "Software\Microsoft\Windows\CurrentVersion\Uninstall\GraylogCollectorSidecar" \
                 "UninstallString" "$\"$INSTDIR\uninstall.exe$\""
  WriteRegStr HKLM "Software\Microsoft\Windows\CurrentVersion\Uninstall\GraylogCollectorSidecar" \
                 "DisplayIcon" "$\"$INSTDIR\graylog.ico$\""				 
  WriteRegStr HKLM "Software\Microsoft\Windows\CurrentVersion\Uninstall\GraylogCollectorSidecar" \
                 "QuietUninstallString" "$\"$INSTDIR\uninstall.exe$\" /S"				 
  WriteRegStr HKLM "Software\Microsoft\Windows\CurrentVersion\Uninstall\GraylogCollectorSidecar" \
                 "DisplayVersion" "${VERSION}"
  WriteRegStr HKLM "Software\Microsoft\Windows\CurrentVersion\Uninstall\GraylogCollectorSidecar" \
                 "InstallLocation" "$INSTDIR"
  WriteRegStr HKLM "Software\Microsoft\Windows\CurrentVersion\Uninstall\GraylogCollectorSidecar" \
                 "RegCompany" "Graylog, Inc."
  WriteRegStr HKLM "Software\Microsoft\Windows\CurrentVersion\Uninstall\GraylogCollectorSidecar" \
                 "Publisher" "Graylog, Inc."
  WriteRegStr HKLM "Software\Microsoft\Windows\CurrentVersion\Uninstall\GraylogCollectorSidecar" \
                 "HelpLink" "https://www.graylog.org"
				 
  WriteRegDWORD HKLM "Software\Microsoft\Windows\CurrentVersion\Uninstall\GraylogCollectorSidecar" \
                 "NoModify" "1"
  WriteRegDWORD HKLM "Software\Microsoft\Windows\CurrentVersion\Uninstall\GraylogCollectorSidecar" \
                 "NoRepair" "1"				 
  WriteRegDWORD HKLM "Software\Microsoft\Windows\CurrentVersion\Uninstall\GraylogCollectorSidecar" \
                 "EstimatedSize" "25000"


SectionEnd
 
Section "Post"

  ; Update configuration
  ${GetParameters} $Params
  ${GetOptions} $Params "-SERVERURL=" $ParamServerUrl
  ${GetOptions} $Params "-TAGS=" $ParamTags

  ${If} $ParamServerUrl != ""
    StrCpy $ServerUrl $ParamServerUrl
  ${EndIf}
  ${If} $ParamTags != ""
    StrCpy $0 $ParamTags
    Loop_Tags:
      ${WordFind} $0 " " "+1" $1
      ${If} $Tags == ""
        StrCpy $Tags $1
      ${Else}
        StrCpy $Tags `$Tags, $1`
      ${EndIf}

      ${WordFind2X} $0 $1 " " "-1}}" $0
      StrCmp $0 $1 Loop_End Loop_Tags

    Loop_End:
  ${EndIf}

  ; default for silent install
  ${If} $ServerUrl == ""
    StrCpy $ServerUrl "http://127.0.0.1:9000/api"
  ${EndIf}
  ${If} $Tags == ""
    StrCpy $Tags "windows, iis"
  ${EndIf}

  !insertmacro _ReplaceInFile "$INSTDIR\collector_sidecar.yml" "<SERVERURL>" $ServerUrl
  !insertmacro _ReplaceInFile "$INSTDIR\collector_sidecar.yml" "<TAGS>" `[$Tags]`

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
 
  ;Remove uninstall entries in the registry 
  DeleteRegKey HKLM "Software\Microsoft\Windows\CurrentVersion\Uninstall\GraylogCollectorSidecar"	
 
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
  ${NSD_CreateText} 50 20 75% 12u "http://127.0.0.1:9000/api"
  Pop $InputServerUrl

  ${NSD_CreateLabel} 0 50 100% 12u "Please enter the configuration tags this host should receive:"
  Pop $Label
  ${NSD_CreateText} 50 70 75% 12u "windows, iis"
  Pop $InputTags

  nsDialogs::Show
FunctionEnd

Function nsDialogsPageLeave
  ${NSD_GetText} $InputServerUrl $ServerUrl
  ${NSD_GetText} $InputTags $Tags

  ${If} $ServerUrl == ""
      MessageBox MB_OK "Please enter a valid address to your Graylog server!"
      Abort
  ${EndIf}
  ${If} $Tags == ""
      MessageBox MB_OK "Please enter one or more tags!"
      Abort
  ${EndIf}
FunctionEnd
