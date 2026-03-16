; Shuttle NSIS Installer Script
; Usage: makensis shuttle.nsi

!define PRODUCT_NAME "Shuttle"
!define PRODUCT_VERSION "${VERSION}"
!define PRODUCT_PUBLISHER "Shuttle"
!define PRODUCT_WEB_SITE "https://github.com/shuttleX/shuttle"

!include "MUI2.nsh"

Name "${PRODUCT_NAME} ${PRODUCT_VERSION}"
OutFile "dist\Shuttle-${PRODUCT_VERSION}-Setup.exe"
InstallDir "$PROGRAMFILES\Shuttle"
RequestExecutionLevel admin

; UI
!define MUI_ABORTWARNING
!insertmacro MUI_PAGE_WELCOME
!insertmacro MUI_PAGE_DIRECTORY
!insertmacro MUI_PAGE_INSTFILES
!insertmacro MUI_PAGE_FINISH
!insertmacro MUI_UNPAGE_CONFIRM
!insertmacro MUI_UNPAGE_INSTFILES
!insertmacro MUI_LANGUAGE "English"
!insertmacro MUI_LANGUAGE "SimpChinese"

Section "Install"
  SetOutPath "$INSTDIR"

  ; Binaries
  File "dist\shuttle.exe"
  File "dist\shuttled.exe"
  File "dist\shuttle-gui.exe"

  ; Example configs
  File "config\client.example.yaml"
  File "config\server.example.yaml"

  ; Create uninstaller
  WriteUninstaller "$INSTDIR\uninstall.exe"

  ; Start menu shortcuts
  CreateDirectory "$SMPROGRAMS\Shuttle"
  CreateShortcut "$SMPROGRAMS\Shuttle\Shuttle.lnk" "$INSTDIR\shuttle-gui.exe"
  CreateShortcut "$SMPROGRAMS\Shuttle\Uninstall.lnk" "$INSTDIR\uninstall.exe"

  ; Desktop shortcut
  CreateShortcut "$DESKTOP\Shuttle.lnk" "$INSTDIR\shuttle-gui.exe"

  ; Register URI handler
  WriteRegStr HKCR "shuttle" "" "URL:Shuttle Protocol"
  WriteRegStr HKCR "shuttle" "URL Protocol" ""
  WriteRegStr HKCR "shuttle\shell\open\command" "" '"$INSTDIR\shuttle-gui.exe" "%1"'

  ; Add to PATH
  EnVar::AddValue "PATH" "$INSTDIR"

  ; Registry for uninstall
  WriteRegStr HKLM "Software\Microsoft\Windows\CurrentVersion\Uninstall\Shuttle" "DisplayName" "${PRODUCT_NAME}"
  WriteRegStr HKLM "Software\Microsoft\Windows\CurrentVersion\Uninstall\Shuttle" "UninstallString" "$INSTDIR\uninstall.exe"
  WriteRegStr HKLM "Software\Microsoft\Windows\CurrentVersion\Uninstall\Shuttle" "Publisher" "${PRODUCT_PUBLISHER}"
  WriteRegStr HKLM "Software\Microsoft\Windows\CurrentVersion\Uninstall\Shuttle" "DisplayVersion" "${PRODUCT_VERSION}"
SectionEnd

Section "Uninstall"
  Delete "$INSTDIR\shuttle.exe"
  Delete "$INSTDIR\shuttled.exe"
  Delete "$INSTDIR\shuttle-gui.exe"
  Delete "$INSTDIR\client.example.yaml"
  Delete "$INSTDIR\server.example.yaml"
  Delete "$INSTDIR\uninstall.exe"
  RMDir "$INSTDIR"

  Delete "$SMPROGRAMS\Shuttle\Shuttle.lnk"
  Delete "$SMPROGRAMS\Shuttle\Uninstall.lnk"
  RMDir "$SMPROGRAMS\Shuttle"
  Delete "$DESKTOP\Shuttle.lnk"

  DeleteRegKey HKCR "shuttle"
  DeleteRegKey HKLM "Software\Microsoft\Windows\CurrentVersion\Uninstall\Shuttle"
  EnVar::DeleteValue "PATH" "$INSTDIR"
SectionEnd
