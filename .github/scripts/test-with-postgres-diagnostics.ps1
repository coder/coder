param(
    [string] $OutputDirectory,
    [string] $DatabasePath,
    [ValidateSet('test', 'test-race')][string] $Target = 'test'
)

# Native make failures must survive diagnostic startup and cleanup errors.
$PSNativeCommandUseErrorActionPreference = $false
$testExit = 1
$sampler = $null
$errorFile = Join-Path $OutputDirectory 'sampler-errors.jsonl'
function Write-SamplerError {
    param($Exception)
    # Do not persist exception messages, which may include runner secrets.
    $record = @{
        timestamp = [DateTime]::UtcNow.ToString('o')
        type = $Exception.GetType().FullName
        hresult = $Exception.HResult
    } | ConvertTo-Json -Compress
    try { Add-Content -LiteralPath $errorFile -Value $record -ErrorAction Stop } catch {
        Write-Warning 'Could not write PostgreSQL sampler diagnostics.'
    }
}
try {
    try {
        $null = New-Item -ItemType Directory -Path $OutputDirectory -Force -ErrorAction Stop
        $parent = Get-Process -Id $PID -ErrorAction Stop
        # EncodedCommand avoids Start-Process joining paths containing
        # spaces into ambiguous arguments. All values are literal strings.
        $arguments = @((Join-Path $PSScriptRoot 'postgres-diagnostics.ps1'), (Join-Path $OutputDirectory 'samples.jsonl'), $DatabasePath, $errorFile) |
            ForEach-Object { "'" + $_.Replace("'", "''") + "'" }
        $command = @"
`$ErrorActionPreference = 'Stop'
try {
    & $($arguments[0]) -OutputFile $($arguments[1]) -ParentId $PID -ParentStartTicks $($parent.StartTime.ToUniversalTime().Ticks) -DatabasePath $($arguments[2])
} catch {
    @{ timestamp = [DateTime]::UtcNow.ToString('o'); type = `$_.Exception.GetType().FullName; hresult = `$_.Exception.HResult } | ConvertTo-Json -Compress | Add-Content -LiteralPath $($arguments[3])
    exit 1
}
"@
        $encoded = [Convert]::ToBase64String([Text.Encoding]::Unicode.GetBytes($command))
        $sampler = Start-Process -FilePath $parent.Path -ArgumentList @(
            '-NoProfile', '-NonInteractive', '-EncodedCommand', $encoded
        ) -PassThru -NoNewWindow -ErrorAction Stop
    } catch {
        Write-SamplerError $_.Exception
        Write-Warning 'PostgreSQL sampler did not start; running tests without it.'
    }
    # Only make inherits the original caller path; the sampler keeps PS7's path.
    $modulePath = $env:PSModulePath
    try {
        switch ($env:CODER_TEST_PSMODULEPATH_PRESENT) {
            'present' { $env:PSModulePath = $env:CODER_TEST_PSMODULEPATH }
            'absent' { $env:PSModulePath = $null }
        }
        & make $Target
        $testExit = $LASTEXITCODE
    } finally {
        $env:PSModulePath = $modulePath
    }
} finally {
    if ($null -ne $sampler) {
        try {
            # Keep the original process handle, never look up a PID or kill
            # a process tree. A blocked collector must not delay test exit.
            if (-not $sampler.HasExited) { $sampler.Kill() }
            if (-not $sampler.WaitForExit(2000)) {
                Write-SamplerError ([TimeoutException]::new())
            }
        } catch {
            Write-SamplerError $_.Exception
        } finally {
            $sampler.Dispose()
        }
    }
}
exit $testExit
