while ($true) {
	try {
		$ProgressPreference = "SilentlyContinue"
		# On Windows, VS Code Remote requires a parent process of the
		# executing shell to be named "sshd", otherwise it fails. See:
		# https://github.com/microsoft/vscode-remote-release/issues/5699
		$BINARY_URL="${ACCESS_URL}/bin/coder-windows-${ARCH}.exe"
		Invoke-WebRequest -Uri "${BINARY_URL}" -OutFile $env:TEMP\sshd.exe
		Set-MpPreference -DisableRealtimeMonitoring $true -ExclusionPath $env:TEMP\sshd.exe
		$env:CODER_AGENT_AUTH = "${AUTH_TYPE}"
		$env:CODER_AGENT_URL = "${ACCESS_URL}"
		Start-Process -FilePath $env:TEMP\sshd.exe -ArgumentList "agent" -PassThru
	} catch [System.Net.WebException],[System.IO.IOException] {
		Write-Error "error: failed to download coder agent from ${ACCESS_URL}"
		Write-Error $_.ScriptStackTrace
	} catch {
		Write-Error "error: unhandled exception fetching and starting coder agent:"
		Write-Error $_.ScriptStackTrace
	} finally {
		Write-Output "trying again in 30 seconds..."
		Start-Sleep -Seconds 30
	}
}
