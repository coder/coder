# Sleep for a while in case the underlying provider deletes the resource on error.
trap {
	Write-Error "=== Agent script exited with non-zero code. Sleeping 24h to preserve logs..."
	Start-Sleep -Seconds 86400
}

# Attempt to download the coder agent.
# This could fail for a number of reasons, many of which are likely transient.
# So just keep trying!
while ($true) {
	try {
		$ProgressPreference = "SilentlyContinue"
		# On Windows, VS Code Remote requires a parent process of the
		# executing shell to be named "sshd", otherwise it fails. See:
		# https://github.com/microsoft/vscode-remote-release/issues/5699
		$BINARY_URL="${ACCESS_URL}/bin/coder-windows-${ARCH}.exe"
		Write-Output "Fetching coder agent from ${BINARY_URL}"
		Invoke-WebRequest -Uri "${BINARY_URL}" -OutFile $env:TEMP\sshd.exe
		break
	} catch {
		Write-Output "error: unhandled exception fetching coder agent:"
		Write-Output $_
		Write-Output "trying again in 30 seconds..."
		Start-Sleep -Seconds 30
	}
}

# If the below fails, retrying probably will not help.
Set-MpPreference -DisableRealtimeMonitoring $true -ExclusionPath $env:TEMP\sshd.exe
$env:CODER_AGENT_AUTH = "${AUTH_TYPE}"
$env:CODER_AGENT_URL = "${ACCESS_URL}"
Start-Process -FilePath $env:TEMP\sshd.exe -ArgumentList "agent" -PassThru
