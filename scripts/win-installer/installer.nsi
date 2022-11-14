# This NSIS installer script was taken from the following webpage and heavily
# adapted to Coder's needs:
# https://www.conjur.org/blog/building-a-windows-installer-from-a-linux-ci-pipeline/

# Since we only build an AMD64 installer for now, ensure that the generated
# installer matches so wingetcreate can sniff the architecture properly.
CPU amd64
Unicode true

!define APP_NAME "Coder"
!define COMP_NAME "Coder Technologies, Inc."
!define VERSION "${CODER_NSIS_VERSION}"
!define COPYRIGHT "Copyright (c) ${CODER_YEAR} Coder Technologies, Inc."
!define DESCRIPTION "Remote development environments on your infrastructure provisioned with Terraform"
!define INSTALLER_NAME "installer.exe"
!define MAIN_APP_EXE "coder.exe"
!define MAIN_APP_EXE_PATH "bin\coder.exe"
!define ICON "coder.ico"
!define BANNER "banner.bmp"
!define LICENSE_TXT "license.txt"

!define INSTALL_DIR "$PROGRAMFILES64\${APP_NAME}"
!define INSTALL_TYPE "SetShellVarContext all" # this means install for all users
!define REG_ROOT "HKLM"
!define REG_APP_PATH "Software\Microsoft\Windows\CurrentVersion\App Paths\${MAIN_APP_EXE}"
!define UNINSTALL_PATH "Software\Microsoft\Windows\CurrentVersion\Uninstall\${APP_NAME}"

######################################################################

VIProductVersion "${VERSION}"
VIAddVersionKey "ProductName" "${APP_NAME}"
VIAddVersionKey "CompanyName" "${COMP_NAME}"
VIAddVersionKey "LegalCopyright" "${COPYRIGHT}"
VIAddVersionKey "FileDescription" "${DESCRIPTION}"
VIAddVersionKey "FileVersion" "${VERSION}"

######################################################################

SetCompressor /SOLID Lzma
Name "${APP_NAME}"
Caption "${APP_NAME}"
OutFile "${INSTALLER_NAME}"
BrandingText "${APP_NAME} v${CODER_VERSION}"
InstallDirRegKey "${REG_ROOT}" "${REG_APP_PATH}" "Path"
InstallDir "${INSTALL_DIR}"

######################################################################

!define MUI_ICON "${ICON}"
!define MUI_UNICON "${ICON}"
!define MUI_WELCOMEFINISHPAGE_BITMAP "${BANNER}"
!define MUI_UNWELCOMEFINISHPAGE_BITMAP "${BANNER}"

######################################################################

!include "MUI2.nsh"

!define MUI_ABORTWARNING
!define MUI_UNABORTWARNING

!define MUI_WELCOMEPAGE_TEXT "Setup will guide you through the installation of Coder v${CODER_VERSION}.$\r$\n$\r$\nClick Next to continue."

!insertmacro MUI_PAGE_WELCOME

!insertmacro MUI_PAGE_LICENSE "${LICENSE_TXT}"

!insertmacro MUI_PAGE_COMPONENTS

!insertmacro MUI_PAGE_DIRECTORY

!insertmacro MUI_PAGE_INSTFILES

!define MUI_FINISHPAGE_TEXT "Coder v${CODER_VERSION} has been installed on your computer.$\r$\n$\r$\nIf you added Coder to your PATH, you can use Coder by opening a command prompt or PowerShell and running `coder`. You may have to sign out and sign back in for `coder` to be available.$\r$\n$\r$\nClick Finish to close Setup."

!insertmacro MUI_PAGE_FINISH

!insertmacro MUI_UNPAGE_CONFIRM

!insertmacro MUI_UNPAGE_INSTFILES

!insertmacro MUI_UNPAGE_FINISH

!insertmacro MUI_LANGUAGE "English"

######################################################################

!include ".\path.nsh"

Section "Coder CLI" SecInstall
	SectionIn RO # mark this section as required

	${INSTALL_TYPE}

	SetOverwrite ifnewer
	SetOutPath "$INSTDIR"
	File /r "bin"
	File "${LICENSE_TXT}"

	WriteUninstaller "$INSTDIR\uninstall.exe"

	WriteRegStr ${REG_ROOT} "${REG_APP_PATH}" "" "$INSTDIR\${MAIN_APP_EXE_PATH}"
	WriteRegStr ${REG_ROOT} "${REG_APP_PATH}" "Path" "$INSTDIR"
	WriteRegStr ${REG_ROOT} "${UNINSTALL_PATH}" "DisplayName" "${APP_NAME}"
	WriteRegStr ${REG_ROOT} "${UNINSTALL_PATH}" "UninstallString" "$INSTDIR\uninstall.exe"
	WriteRegStr ${REG_ROOT} "${UNINSTALL_PATH}" "DisplayIcon" "$INSTDIR\${MAIN_APP_EXE_PATH}"
	WriteRegStr ${REG_ROOT} "${UNINSTALL_PATH}" "DisplayVersion" "${VERSION}"
	WriteRegStr ${REG_ROOT} "${UNINSTALL_PATH}" "Publisher" "${COMP_NAME}"
SectionEnd

Section "Add to PATH" SecAddToPath
	Push "$INSTDIR\bin"
	Call AddToPath
SectionEnd

######################################################################

Section Uninstall
	${INSTALL_TYPE}

	RmDir /r "$INSTDIR"
	DeleteRegKey ${REG_ROOT} "${REG_APP_PATH}"
	DeleteRegKey ${REG_ROOT} "${UNINSTALL_PATH}"

	Push "$INSTDIR\bin"
	Call un.RemoveFromPath
SectionEnd

######################################################################

LangString DESC_SecInstall ${LANG_ENGLISH} "Install the Coder command-line interface (coder.exe) for all users."
LangString DESC_SecAddToPath ${LANG_ENGLISH} "Add coder.exe to the PATH for all users. This enables `coder` to be used directly from a command prompt or PowerShell."

!insertmacro MUI_FUNCTION_DESCRIPTION_BEGIN
  !insertmacro MUI_DESCRIPTION_TEXT ${SecInstall} $(DESC_SecInstall)
  !insertmacro MUI_DESCRIPTION_TEXT ${SecAddToPath} $(DESC_SecAddToPath)
!insertmacro MUI_FUNCTION_DESCRIPTION_END
