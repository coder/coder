package usershell

import "os/exec"

// get resolves the Windows shell, preferring pwsh.exe, then
// powershell.exe, then cmd.exe. It backs SystemEnvInfo.Shell. Callers
// resolve the shell through an EnvInfoer.
func get(username string) (string, error) {
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
