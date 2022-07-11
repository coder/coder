# On Windows, VS Code Remote requires a parent process of the
# executing shell to be named "sshd", otherwise it fails. See:
# https://github.com/microsoft/vscode-remote-release/issues/5699
$ProgressPreference = "SilentlyContinue"
Invoke-WebRequest -Uri ${ACCESS_URL}bin/coder-windows-${ARCH}.exe -OutFile $env:TEMP\sshd.exe
Set-MpPreference -DisableRealtimeMonitoring $true -ExclusionPath $env:TEMP\sshd.exe
$env:CODER_AGENT_AUTH = "${AUTH_TYPE}"
$env:CODER_AGENT_URL = "${ACCESS_URL}"
Start-Process -FilePath $env:TEMP\sshd.exe -ArgumentList "agent" -PassThru
