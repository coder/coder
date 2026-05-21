package usershell

import "os/exec"

// Get returns the command prompt binary name.
// Deprecated: use SystemEnvInfo.UserShell instead.
func Get(username string) (string, error) {
	_, err := exec.LookPath("pwsh.exe")
	if err == nil {
		return "pwsh.exe", nil
	}
	_, err = exec.LookPath("powershell.exe")
	if err == nil {
		return "powershell.exe", nil
	}
	return "cmd.exe", nil
}
