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
		Write-Output "$(Get-Date) Fetching coder agent from ${BINARY_URL}"
		Invoke-WebRequest -Uri "${BINARY_URL}" -OutFile $env:TEMP\sshd.exe
		break
	} catch {
		Write-Output "$(Get-Date) error: unhandled exception fetching coder agent:"
		Write-Output $_
		Write-Output "$(Get-Date) trying again in 30 seconds..."
		Start-Sleep -Seconds 30
	}
}

# Check if running in a Windows container
if (-not (Get-Command 'Set-MpPreference' -ErrorAction SilentlyContinue)) {
    Write-Output "$(Get-Date) Set-MpPreference not available, skipping..."
} else {
    Set-MpPreference -DisableRealtimeMonitoring $true -ExclusionPath $env:TEMP\sshd.exe
}

$env:CODER_AGENT_AUTH = "${AUTH_TYPE}"
$env:CODER_AGENT_URL = "${ACCESS_URL}"

$psi = [System.Diagnostics.ProcessStartInfo]::new("$env:TEMP\sshd.exe", '--version')
$psi.UseShellExecute = $false
$psi.RedirectStandardOutput = $true
$p = [System.Diagnostics.Process]::Start($psi)
$output = $p.StandardOutput.ReadToEnd()
$p.WaitForExit()

if ($output -notlike "*Coder*") {
  Write-Output "$env:TEMP\sshd.exe --version output: `"$output"`"
  Write-Error "ERROR: Downloaded agent binary returned unexpected version output"
  Throw "unexpected binary"
}

# Check if we're running inside a Windows container!
$inContainer = $false
if ((Get-ItemProperty 'HKLM:\SYSTEM\CurrentControlSet\Control' -Name 'ContainerType' -ErrorAction SilentlyContinue) -ne $null) {
    $inContainer = $true
}
if ($inContainer) {
    # If we're in a container, run in a the foreground!
    Start-Process -FilePath $env:TEMP\sshd.exe -ArgumentList "agent" -Wait -NoNewWindow
} else {
    Start-Process -FilePath $env:TEMP\sshd.exe -ArgumentList "agent" -PassThru
}
