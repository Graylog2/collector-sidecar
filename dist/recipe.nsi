; -------------------------------
; Start
 
  Name "Graylog Sidecar"
  !define MUI_FILE "savefile"
  !define MUI_BRANDINGTEXT "Graylog Sidecar v${VERSION}${VERSION_SUFFIX}"
  CRCCheck On
  SetCompressor "bzip2"
 
  !include "${NSISDIR}\Contrib\Modern UI\System.nsh"
  !include nsDialogs.nsh
  !include LogicLib.nsh
  !include StrRep.nsh
  !include ReplaceInFile.nsh
  !include Common.nsh
  !include FileFunc.nsh
  !include WordFunc.nsh
  !include x64.nsh
  !include IfKeyExists.nsh

  VIProductVersion "${VERSION}.${REVISION}" ;Required format is X.X.X.X
  VIAddVersionKey "FileVersion" "${VERSION}"
  VIAddVersionKey "FileDescription" "Graylog Sidecar Installer"
  VIAddVersionKey "ProductName" "Graylog Sidecar"
  VIAddVersionKey "ProductVersion" "${VERSION}${VERSION_SUFFIX}"
  VIAddVersionKey "LegalCopyright" "Graylog, Inc."
 
;---------------------------------
;General

  !searchreplace SUFFIX '${VERSION_SUFFIX}' "-" "."
  OutFile "pkg/graylog_sidecar_installer_${VERSION}-${REVISION}${SUFFIX}.exe"
  RequestExecutionLevel admin ;Require admin rights
  ShowInstDetails "show"
  ShowUninstDetails "show"

  ; Variables
  Var Params
  Var ParamServerUrl
  Var InputServerUrl
  Var ServerUrl
  Var ParamNodeName
  Var InputNodeName
  Var InputApiToken
  Var NodeName
  Var ApiToken
  Var ParamUpdateInterval
  Var UpdateInterval
  Var ParamTlsSkipVerify
  Var TlsSkipVerify
  Var ParamSendStatus
  Var ParamApiToken
  Var ParamNodeId
  Var NodeId
  Var SendStatus
  Var Dialog
  Var Label
  Var GraylogDir
  Var IsUpgrade
  Var LogFile
  Var LogMsgText


;--------------------------------
;Modern UI Configuration  
  
  !define MUI_ICON "graylog.ico"  
  !define MUI_WELCOMEPAGE_TITLE "Graylog Sidecar ${VERSION}-${REVISION}${SUFFIX} Installation / Upgrade"
  !define MUI_WELCOMEPAGE_TEXT  "This setup is gonna guide you through the installation / upgrade of the Graylog Sidecar.\r\n\r\n \
		  If an already configured Sidecar is detected ('sidecar.yml' present), it will perform an upgrade.\r\n \r\n\
		  Click Next to continue."

  !insertmacro MUI_PAGE_WELCOME
  !insertmacro MUI_PAGE_LICENSE  "../LICENSE"
  !insertmacro MUI_UNPAGE_WELCOME
  !insertmacro MUI_UNPAGE_CONFIRM
  !insertmacro MUI_UNPAGE_INSTFILES

  
  ; Custom Pages
  Page custom nsDialogsPage nsDialogsPageLeave
  Page instfiles

  !insertmacro MUI_PAGE_FINISH
  !insertmacro MUI_UNPAGE_FINISH
  !define MUI_DIRECTORYPAGE
  !define MUI_ABORTWARNING
 
;--------------------------------
;Macros
 
  !insertmacro MUI_LANGUAGE "English"
  !insertmacro WordFind
  !insertmacro WordFind2X
  !insertmacro GetTime

  !macro _LogWrite text
    StrCpy $LogMsgText "${text}"
    ${GetTime} "" "L" $0 $1 $2 $3 $4 $5 $6
    FileWrite $LogFile '$2$1$0$4$5$6: $LogMsgText$\r$\n'
    System::Call 'kernel32::GetStdHandle(i -11)i.r9'
    System::Call 'kernel32::AttachConsole(i -1)'
    FileWrite $9 "$LogMsgText$\r$\n"
  !macroend
  !define LogWrite "!insertmacro _LogWrite"


  !macro Check_X64
    ${If} ${RunningX64}
      SetRegView 64
      Strcpy $GraylogDir "$PROGRAMFILES64\Graylog"
    ${Else}
      SetRegView 32
      Strcpy $GraylogDir "$PROGRAMFILES32\Graylog"
    ${EndIf}
    Strcpy $INSTDIR "$GraylogDir\sidecar"
    CreateDirectory $INSTDIR
  !macroend

  !macro Check_Upgrade
    ${If} ${FileExists} "$INSTDIR\sidecar.yml"
      Strcpy $IsUpgrade "true"
      ${LogWrite} "Existing installation detected. Performing upgrade."
    ${Else}
      Strcpy $IsUpgrade "false"
      ${LogWrite} "No previous installation detected. Running installation mode."
    ${EndIf}
  !macroend


;--------------------------------
;Data
 
  LicenseData "../LICENSE"

;-------------------------------- 
;Installer Sections     
Section "Install"

  ;These folders are needed at runtime
  CreateDirectory "$INSTDIR\generated"
  CreateDirectory "$INSTDIR\logs"
  CreateDirectory "$INSTDIR\module"
  SetOutPath "$INSTDIR"
 
  SetOverwrite off
  File /oname=sidecar.yml "../sidecar-windows-example.yml"
  SetOverwrite on
  File /oname=sidecar.yml.dist "../sidecar-windows-example.yml"
  File "../LICENSE"
  File "graylog.ico"  

  ;Stop service to allow binary upgrade
  !insertmacro _IfKeyExists HKLM "SYSTEM\CurrentControlSet\Services" "graylog-sidecar"
  Pop $R0
  ${If} $R0 = 1
    nsExec::ExecToStack '"$INSTDIR\graylog-sidecar.exe" -service stop'
    Pop $0
    Pop $1
    ${LogWrite} "Stopping existing Sidecar Service: [exit $0] Stdout: $1"
  ${EndIf}

  ${If} ${RunningX64}
    File /oname=graylog-sidecar.exe "../build/${VERSION}/windows/amd64/graylog-sidecar.exe"
  ${Else}
    File /oname=graylog-sidecar.exe "../build/${VERSION}/windows/386/graylog-sidecar.exe"
  ${EndIf}

  ; Install beats collectors
  ${If} ${RunningX64}
    File "collectors/winlogbeat/windows/x86_64/winlogbeat.exe"
    File "collectors/filebeat/windows/x86_64/filebeat.exe"
  ${Else}
    File "collectors/winlogbeat/windows/x86/winlogbeat.exe"
    File "collectors/filebeat/windows/x86/filebeat.exe"
  ${EndIf}

  ;When we stop the Sidecar service we also turn it on again
  ${If} $R0 = 1
    nsExec::ExecToStack '"$INSTDIR\graylog-sidecar.exe" -service start'
    Pop $0
    Pop $1
    ${LogWrite} "Restarting existing Sidecar Service: [exit $0] Stdout: $1"
  ${EndIf}

  WriteUninstaller "$INSTDIR\uninstall.exe"

  WriteRegStr HKLM "Software\Microsoft\Windows\CurrentVersion\Uninstall\GraylogSidecar" \
                 "DisplayName" "Graylog Sidecar"
  WriteRegStr HKLM "Software\Microsoft\Windows\CurrentVersion\Uninstall\GraylogSidecar" \
                 "UninstallString" "$\"$INSTDIR\uninstall.exe$\""
  WriteRegStr HKLM "Software\Microsoft\Windows\CurrentVersion\Uninstall\GraylogSidecar" \
                 "DisplayIcon" "$\"$INSTDIR\graylog.ico$\""				 
  WriteRegStr HKLM "Software\Microsoft\Windows\CurrentVersion\Uninstall\GraylogSidecar" \
                 "QuietUninstallString" "$\"$INSTDIR\uninstall.exe$\" /S"				 
  WriteRegStr HKLM "Software\Microsoft\Windows\CurrentVersion\Uninstall\GraylogSidecar" \
                 "DisplayVersion" "${VERSION}${VERSION_SUFFIX}"
  WriteRegStr HKLM "Software\Microsoft\Windows\CurrentVersion\Uninstall\GraylogSidecar" \
                 "InstallLocation" "$INSTDIR"
  WriteRegStr HKLM "Software\Microsoft\Windows\CurrentVersion\Uninstall\GraylogSidecar" \
                 "RegCompany" "Graylog, Inc."
  WriteRegStr HKLM "Software\Microsoft\Windows\CurrentVersion\Uninstall\GraylogSidecar" \
                 "Publisher" "Graylog, Inc."
  WriteRegStr HKLM "Software\Microsoft\Windows\CurrentVersion\Uninstall\GraylogSidecar" \
                 "HelpLink" "https://www.graylog.org"
				 
  WriteRegDWORD HKLM "Software\Microsoft\Windows\CurrentVersion\Uninstall\GraylogSidecar" \
                 "NoModify" "1"
  WriteRegDWORD HKLM "Software\Microsoft\Windows\CurrentVersion\Uninstall\GraylogSidecar" \
                 "NoRepair" "1"				 
  WriteRegDWORD HKLM "Software\Microsoft\Windows\CurrentVersion\Uninstall\GraylogSidecar" \
                 "EstimatedSize" "25000"


SectionEnd
 
Section "Post"

  ; Parse command line options
  ; The first character in the option string is treated as a parameter delimiter, so we prepend a white-space
  ; to allow options like -NODENAME=my-collector (second dash would interrupt option parsing otherwise)
  ${GetParameters} $Params
  ${GetOptions} $Params " -SERVERURL=" $ParamServerUrl
  ${GetOptions} $Params " -NODENAME=" $ParamNodeName
  ${GetOptions} $Params " -UPDATE_INTERVAL=" $ParamUpdateInterval
  ${GetOptions} $Params " -TLS_SKIP_VERIFY=" $ParamTlsSkipVerify
  ${GetOptions} $Params " -SEND_STATUS=" $ParamSendStatus
  ${GetOptions} $Params " -APITOKEN=" $ParamApiToken
  ${GetOptions} $Params " -NODEID=" $ParamNodeId

  ${If} $ParamServerUrl != ""
    StrCpy $ServerUrl $ParamServerUrl
  ${EndIf}
  ${If} $ParamNodeName != ""
    StrCpy $NodeName $ParamNodeName
  ${EndIf}
  ${If} $ParamUpdateInterval != ""
    StrCpy $UpdateInterval $ParamUpdateInterval
  ${EndIf}
  ${If} $ParamTlsSkipVerify != ""
    StrCpy $TlsSkipVerify $ParamTlsSkipVerify
  ${EndIf}
  ${If} $ParamSendStatus != ""
    StrCpy $SendStatus $ParamSendStatus
  ${EndIf}
  ${If} $ParamApiToken != ""
    StrCpy $ApiToken $ParamApiToken
  ${EndIf}
  ${If} $ParamNodeId != ""
    StrCpy $NodeId $ParamNodeId
  ${EndIf}

  ; set defaults
  ${If} $ServerUrl == ""
    StrCpy $ServerUrl "http://127.0.0.1:9000/api"
  ${EndIf}
  ${If} $UpdateInterval == ""
    StrCpy $UpdateInterval "10"
  ${EndIf}
  ${If} $TlsSkipVerify == ""
    StrCpy $TlsSkipVerify "false"
  ${EndIf}
  ${If} $SendStatus == ""
    StrCpy $SendStatus "true"
  ${EndIf}
  ${If} $NodeId == ""
    ;sidecar.yml needs double escapes
    ${WordReplace} "file:$INSTDIR\node-id" "\" "\\" "+" $NodeId
  ${EndIf}

  !insertmacro _ReplaceInFile "$INSTDIR\sidecar.yml" "<SERVERURL>" $ServerUrl
  !insertmacro _ReplaceInFile "$INSTDIR\sidecar.yml" "<NODENAME>" $NodeName
  !insertmacro _ReplaceInFile "$INSTDIR\sidecar.yml" "<UPDATEINTERVAL>" $UpdateInterval
  !insertmacro _ReplaceInFile "$INSTDIR\sidecar.yml" "<TLSSKIPVERIFY>" $TlsSkipVerify
  !insertmacro _ReplaceInFile "$INSTDIR\sidecar.yml" "<SENDSTATUS>" $SendStatus
  !insertmacro _ReplaceInFile "$INSTDIR\sidecar.yml" "<APITOKEN>" $ApiToken
  !insertmacro _ReplaceInFile "$INSTDIR\sidecar.yml" "<NODEID>" $NodeId

  ;Install sidecar service
  ${If} $IsUpgrade == 'false'
    nsExec::ExecToStack '"$INSTDIR\graylog-sidecar.exe" -service install'
    Pop $0
    Pop $1
    ${LogWrite} "Installing new Sidecar Service: [exit $0] Stdout: $1"

    nsExec::ExecToStack '"$INSTDIR\graylog-sidecar.exe" -service start'
    Pop $0
    Pop $1
    ${LogWrite} "Starting new Sidecar Service: [exit $0] Stdout: $1"
  ${EndIf}

  ${LogWrite} "Installer/Upgrader finished."
  FileClose $LogFile
SectionEnd
 
;--------------------------------    
;Uninstaller Section  
Section "Uninstall"

  ;Uninstall system service
  ExecWait '"$INSTDIR\graylog-sidecar.exe" -service stop'
  ExecWait '"$INSTDIR\graylog-sidecar.exe" -service uninstall'
 
  ;Delete Files
  RMDir /r "$INSTDIR\*.*"    
 
  ;Remove the installation directory
  SetOutPath $TEMP
  RMDir "$INSTDIR"
  RMDir $GraylogDir
 
  ;Remove uninstall entries in the registry 
  DeleteRegKey HKLM "Software\Microsoft\Windows\CurrentVersion\Uninstall\GraylogSidecar"

SectionEnd
 
 
;--------------------------------    
;Functions

Function .onInit
  !insertmacro Check_X64

  FileOpen $LogFile "$INSTDIR\installerlog.txt" w

  ${LogWrite} "$\r$\n" ;Powershell seems to swallow the first line
  ${LogWrite} "Starting Sidecar ${VERSION}-${REVISION}${SUFFIX} installer/upgrader."

  ; check admin rights
  Call CheckAdmin
  
  ; check concurrent un/installations
  Call CheckConcurrent
    
  !insertmacro Check_Upgrade
FunctionEnd

Function un.oninit
  ; check admin rights
  Call un.CheckAdmin
  
  ; check concurrent un/installations
  Call un.CheckConcurrent

  !insertmacro Check_X64
FunctionEnd

 

Function nsDialogsPage
  ${If} $IsUpgrade == 'true'
    Abort
  ${EndIf}

  nsDialogs::Create 1018

  
  !insertmacro MUI_HEADER_TEXT "${MUI_BRANDINGTEXT} Configuration" "Here you can check and modify the configuration of this agent"
  
  
  Pop $Dialog

  ${If} $Dialog == error
     Abort
  ${EndIf}

  ${NSD_CreateLabel} 0 0 100% 12u "Enter the URL to your Graylog API:"
  Pop $Label
  ${NSD_CreateText} 50 20 75% 12u "http://127.0.0.1:9000/api"
  Pop $InputServerUrl

  ${NSD_CreateLabel} 0 60 100% 12u "The name of this instance:"
  Pop $Label
  ${NSD_CreateText} 50 80 75% 12u ""
  Pop $InputNodeName
  ${NSD_CreateLabel} 50 100 100% 12u "If empty, the hostname will be used."
  Pop $Label

  ${NSD_CreateLabel} 0 140 100% 12u "Enter the server API token:"
  Pop $Label
  ${NSD_CreateText} 50 160 75% 12u ""
  Pop $InputApiToken

  nsDialogs::Show
FunctionEnd

Function nsDialogsPageLeave
  ${NSD_GetText} $InputServerUrl $ServerUrl
  ${NSD_GetText} $InputNodeName $NodeName
  ${NSD_GetText} $InputApiToken $ApiToken

  ${If} $ServerUrl == ""
      MessageBox MB_OK "Please enter a valid address to your Graylog server!"
      Abort
  ${EndIf}
  ${If} $ApiToken == ""
      MessageBox MB_OK "Please enter an API token!"
      Abort
  ${EndIf}
FunctionEnd
