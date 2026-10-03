; DAWGit installer for Windows (per-user, no admin rights needed).
; Built by scripts/build-windows.ps1:
;   makensis /DVERSION=0.1.0 /DDIST=<folder with DAWGit.exe and bin\dawgit.exe> installer.nsi
; Requires WebView2, which ships with Windows 11 and current Windows 10.

Unicode true
!include "MUI2.nsh"
!include "FileFunc.nsh"
!include "LogicLib.nsh"
!include "Sections.nsh"

!ifndef VERSION
  !define VERSION "0.0.0"
!endif
!ifndef DIST
  !error "DIST (folder with the built executables) is required"
!endif

; A build with extensions passes its own name (/DAPP="DAWGit Pro") and
; welcome line (/DTAGLINE=...): it installs next to the public app, with its
; own folder, shortcuts and uninstall entry.
!ifndef APP
  !define APP "DAWGit"
!endif
!ifndef TAGLINE
  !define TAGLINE "Version history and teamwork for your Ableton Live projects."
!endif
!define UNINST_KEY "Software\Microsoft\Windows\CurrentVersion\Uninstall\${APP}"
!define RUN_KEY "Software\Microsoft\Windows\CurrentVersion\Run"

Name "${APP}"
OutFile "${DIST}\${APP}-${VERSION}-setup.exe"
InstallDir "$LOCALAPPDATA\Programs\${APP}"
InstallDirRegKey HKCU "${UNINST_KEY}" "InstallLocation"
RequestExecutionLevel user
SetCompressor /SOLID lzma

VIProductVersion "${VERSION}.0"
VIAddVersionKey "ProductName" "${APP}"
VIAddVersionKey "FileDescription" "${APP} Setup"
VIAddVersionKey "ProductVersion" "${VERSION}"
VIAddVersionKey "FileVersion" "${VERSION}"
VIAddVersionKey "CompanyName" "${APP}"
VIAddVersionKey "LegalCopyright" "(c) 2026 ${APP}"

!define MUI_ICON "icon.ico"
!define MUI_UNICON "icon.ico"
!define MUI_ABORTWARNING
!define MUI_WELCOMEPAGE_TITLE "Welcome to ${APP}"
!define MUI_WELCOMEPAGE_TEXT "${TAGLINE}$\r$\n$\r$\n${APP} runs in the system tray: it tells you when your team commits new versions. It never changes your project files on its own.$\r$\n$\r$\nClick Next to continue."
!define MUI_FINISHPAGE_RUN "$INSTDIR\${APP}.exe"
!define MUI_FINISHPAGE_RUN_TEXT "Open ${APP} now"

!insertmacro MUI_PAGE_WELCOME
!insertmacro MUI_PAGE_COMPONENTS
!insertmacro MUI_PAGE_DIRECTORY
!insertmacro MUI_PAGE_INSTFILES
!insertmacro MUI_PAGE_FINISH
!insertmacro MUI_UNPAGE_CONFIRM
!insertmacro MUI_UNPAGE_COMPONENTS
!insertmacro MUI_UNPAGE_INSTFILES
!insertmacro MUI_LANGUAGE "English"

; An update replaces running executables: close DAWGit first. The window's
; close button only hides it to the tray, so the process is ended.
!macro CloseApp
  nsExec::Exec 'taskkill /F /IM "${APP}.exe"'
  Pop $0
  Sleep 500
!macroend

Section "${APP}" SecApp
  SectionIn RO
  !insertmacro CloseApp
  SetOutPath "$INSTDIR"
  File "${DIST}\${APP}.exe"
  File "icon.ico"
  ; The CLI lives in bin\: file names ignore case, so dawgit.exe and
  ; DAWGit.exe cannot share a folder.
  SetOutPath "$INSTDIR\bin"
  File "${DIST}\bin\dawgit.exe"
  SetOutPath "$INSTDIR"

  CreateDirectory "$SMPROGRAMS\${APP}"
  CreateShortcut "$SMPROGRAMS\${APP}\${APP}.lnk" "$INSTDIR\${APP}.exe"
  ; Earlier versions added a Team Server shortcut; the server is now set up
  ; from the command line only (docs/cli.md).
  Delete "$SMPROGRAMS\${APP}\${APP} Team Server.lnk"

  WriteUninstaller "$INSTDIR\Uninstall ${APP}.exe"
  WriteRegStr HKCU "${UNINST_KEY}" "DisplayName" "${APP}"
  WriteRegStr HKCU "${UNINST_KEY}" "DisplayVersion" "${VERSION}"
  WriteRegStr HKCU "${UNINST_KEY}" "Publisher" "${APP}"
  WriteRegStr HKCU "${UNINST_KEY}" "DisplayIcon" "$INSTDIR\icon.ico"
  WriteRegStr HKCU "${UNINST_KEY}" "InstallLocation" "$INSTDIR"
  WriteRegStr HKCU "${UNINST_KEY}" "UninstallString" '"$INSTDIR\Uninstall ${APP}.exe"'
  WriteRegDWORD HKCU "${UNINST_KEY}" "NoModify" 1
  WriteRegDWORD HKCU "${UNINST_KEY}" "NoRepair" 1
SectionEnd

Section "Start with Windows" SecAutostart
  WriteRegStr HKCU "${RUN_KEY}" "${APP}" '"$INSTDIR\${APP}.exe" --background'
SectionEnd

Section "Desktop shortcut" SecDesktop
  CreateShortcut "$DESKTOP\${APP}.lnk" "$INSTDIR\${APP}.exe"
SectionEnd

; dawgit.exe edits the user's PATH itself (only its own entry): an NSIS
; string is cut at 1024 characters, which would cut a long PATH short.
Section "Command line tool for AI agents" SecPath
  nsExec::Exec '"$INSTDIR\bin\dawgit.exe" path add'
  Pop $0
SectionEnd

; The choice is remembered for updates (and the PATH entry taken off when
; it is turned off).
Section "-PathChoice"
  ${If} ${SectionIsSelected} ${SecPath}
    WriteRegDWORD HKCU "${UNINST_KEY}" "AddToPath" 1
  ${Else}
    nsExec::Exec '"$INSTDIR\bin\dawgit.exe" path remove'
    Pop $0
    WriteRegDWORD HKCU "${UNINST_KEY}" "AddToPath" 0
  ${EndIf}
SectionEnd

; DAWGit updating itself runs this silently (/S): the choices made at the
; first install stay as they are (autostart is the app's setting, the
; desktop shortcut is left alone, PATH as chosen), and /relaunch or
; /relaunch-background opens DAWGit again afterwards (its window, or in the
; tray).
Function .onInit
  ; PATH: as chosen before (installs from before the option count as on).
  ClearErrors
  ReadRegDWORD $0 HKCU "${UNINST_KEY}" "AddToPath"
  ${IfNot} ${Errors}
  ${AndIf} $0 == 0
    !insertmacro UnselectSection ${SecPath}
  ${EndIf}
  ${If} ${Silent}
    SectionSetFlags ${SecAutostart} 0
    SectionSetFlags ${SecDesktop} 0
  ${EndIf}
FunctionEnd

Function .onInstSuccess
  ${GetParameters} $R0
  ClearErrors
  ${GetOptions} $R0 "/relaunch-background" $R1
  IfErrors +3
    Exec '"$INSTDIR\${APP}.exe" --background'
    Return
  ClearErrors
  ${GetOptions} $R0 "/relaunch" $R1
  IfErrors +2
    Exec '"$INSTDIR\${APP}.exe"'
FunctionEnd

!insertmacro MUI_FUNCTION_DESCRIPTION_BEGIN
  !insertmacro MUI_DESCRIPTION_TEXT ${SecApp} "The ${APP} app and the command line tool."
  !insertmacro MUI_DESCRIPTION_TEXT ${SecAutostart} "Recommended: keeps ${APP} in the tray so you hear about new versions from your team."
  !insertmacro MUI_DESCRIPTION_TEXT ${SecDesktop} "Put a ${APP} shortcut on the desktop."
  !insertmacro MUI_DESCRIPTION_TEXT ${SecPath} "Adds dawgit to your PATH, so AI coding agents (Claude Code, Codex, Cursor…) and scripts can save, update and check your projects for you. Works in terminals opened afterwards."
!insertmacro MUI_FUNCTION_DESCRIPTION_END

Section "un.${APP}" UnSecApp
  SectionIn RO
  !insertmacro CloseApp
  nsExec::Exec '"$INSTDIR\bin\dawgit.exe" path remove' ; only our entry
  Pop $0
  Delete "$INSTDIR\${APP}.exe"
  Delete "$INSTDIR\bin\dawgit.exe"
  RMDir "$INSTDIR\bin"
  Delete "$INSTDIR\icon.ico"
  Delete "$INSTDIR\Uninstall ${APP}.exe"
  RMDir "$INSTDIR"
  Delete "$SMPROGRAMS\${APP}\${APP}.lnk"
  Delete "$SMPROGRAMS\${APP}\${APP} Team Server.lnk"
  RMDir "$SMPROGRAMS\${APP}"
  Delete "$DESKTOP\${APP}.lnk"
  DeleteRegValue HKCU "${RUN_KEY}" "${APP}"
  DeleteRegKey HKCU "${UNINST_KEY}"
  ; Projects (.dawgit folders) and server data are always left in place.
SectionEnd

; Off by default: reinstalling keeps your teams and name.
Section /o "un.Remove my settings" UnSecSettings
  RMDir /r "$APPDATA\${APP}"      ; teams, access tokens and keys, your name
  RMDir /r "$APPDATA\${APP}.exe"  ; the app window's saved state (WebView2)
SectionEnd

!insertmacro MUI_UNFUNCTION_DESCRIPTION_BEGIN
  !insertmacro MUI_DESCRIPTION_TEXT ${UnSecApp} "The ${APP} app, the command line tool and their shortcuts."
  !insertmacro MUI_DESCRIPTION_TEXT ${UnSecSettings} "Also forget your teams, access tokens and name on this computer. Your projects, their version history and any team server data are kept."
!insertmacro MUI_UNFUNCTION_DESCRIPTION_END
