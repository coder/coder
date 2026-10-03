# Run with pwsh -NoProfile -File .github/scripts/postgres-diagnostics.tests.ps1.
# -Native also verifies real Windows collection against the started PostgreSQL.
param([switch] $Native, [string] $NativeDatabasePath, [string] $NativeOutputDirectory)
$ErrorActionPreference = 'Stop'
$PSNativeCommandUseErrorActionPreference = $false
. "$PSScriptRoot/postgres-diagnostics.ps1"

function Assert-True {
    param([bool] $Condition, [string] $Message)
    if (-not $Condition) { throw $Message }
}

function Assert-NativeSample {
    param($Sample)
    Assert-True ($null -eq $Sample.error) 'Native sampler returned an error instead of a sample.'
    Assert-True ($Sample.sampler_pid -gt 0 -and $null -ne $Sample.timestamp) 'Native sample identity missing.'
    $null = [DateTime]::Parse($Sample.timestamp)
    $owners = @($Sample.tcp.listeners | Where-Object { $_.address -in @('127.0.0.1', '0.0.0.0', '::') })
    Assert-True ($owners.Count -gt 0 -and $Sample.tcp.state_counts.'2' -gt 0) 'Native PostgreSQL listener missing.'
    $processes = @($Sample.processes.details | Where-Object {
        $_.pid -in $owners.pid -and $_.name -eq 'postgres.exe'
    })
    Assert-True ($processes.Count -gt 0) 'Native listener has no PostgreSQL process identity.'
    foreach ($process in $processes) {
        Assert-True ($process.parent_pid -gt 0 -and $null -ne $process.created) 'Native process parent/creation missing.'
        $null = [DateTime]::Parse($process.created)
        Assert-True ($process.working_set_bytes -gt 0 -and $process.private_bytes -gt 0) 'Native process memory missing.'
    }
    foreach ($fields in @(
        @{ object = $Sample.memory; names = @('available_bytes', 'committed_bytes', 'commit_limit_bytes') },
        @{ object = $Sample.disk; names = @('free_bytes', 'total_bytes') }
    )) {
        foreach ($name in $fields.names) {
            $value = $fields.object.$name
            Assert-True (($value -is [long] -or $value -is [int]) -and $value -ge 0) "Native resource field $name missing or nonnumeric."
        }
    }
    Assert-True ($Sample.memory.commit_limit_bytes -gt 0 -and $Sample.disk.total_bytes -gt 0) 'Native resource capacity missing.'
    Assert-True ($Sample.connect -is [Collections.IDictionary]) 'Native connection result must be one object.'
    Assert-True ($Sample.connect.status -in @('connected', 'error', 'timeout')) 'Native connection result missing.'
    Assert-True ($Sample.connect.elapsed_ms -is [long] -or $Sample.connect.elapsed_ms -is [int]) 'Native probe latency missing.'
    Assert-True ($Sample.connect.elapsed_ms -ge 0 -and $Sample.connect.elapsed_ms -lt 5000) 'Native probe exceeded its bounded completion allowance.'
    if ($Sample.connect.status -ne 'connected') {
        Assert-True ($null -ne $Sample.connect.socket_error -or $null -ne $Sample.connect.error.type) 'Native probe failure detail missing.'
    }
}

$directory = Join-Path ([IO.Path]::GetTempPath()) ("postgres diagnostics ' " + [Guid]::NewGuid().ToString())
$null = New-Item -ItemType Directory -Path $directory
try {
    if ($Native) {
        Assert-True $IsWindows 'Native CIM validation requires Windows.'
        # Use the production launcher with a completed make command so shutdown
        # must produce a final native sample, even when tests finish immediately.
        & {
            function make { $global:LASTEXITCODE = 0 }
            & "$PSScriptRoot/test-with-postgres-diagnostics.ps1" -OutputDirectory $NativeOutputDirectory -DatabasePath $NativeDatabasePath
            Assert-True ($LASTEXITCODE -eq 0) 'Native wrapper lost make exit status.'
        }
        $nativeOutput = Join-Path $NativeOutputDirectory 'samples.jsonl'
        Assert-True (-not (Test-Path (Join-Path $NativeOutputDirectory 'sampler-errors.jsonl'))) 'Native production launcher reported sampler errors.'
        $lines = @(Get-Content -LiteralPath $nativeOutput)
        Assert-True ($lines.Count -gt 0) 'Native production launcher did not write samples.'
        foreach ($line in $lines) { Assert-NativeSample ($line | ConvertFrom-Json -AsHashtable) }
        $nativeSample = $lines[-1] | ConvertFrom-Json -AsHashtable
        Assert-True $nativeSample.final 'Native wrapper did not flush a final sample.'
        Assert-True ($null -eq (Get-Process -Id $nativeSample.sampler_pid -ErrorAction SilentlyContinue)) 'Native sampler survived wrapper exit.'
        Copy-Item -LiteralPath $nativeOutput -Destination (Join-Path $NativeOutputDirectory 'native.jsonl')
        # Only these selected fields reach the success log, not a process list
        # or raw errors. This evidence also appears on green cached runs.
        $evidence = @{
            timestamp = $nativeSample.timestamp
            sampler_pid = $nativeSample.sampler_pid
            memory = $nativeSample.memory
            disk = $nativeSample.disk
            connect = $nativeSample.connect
            listener_pids = @($nativeSample.tcp.listeners.pid)
        }
        Write-Host "PASS: native Windows PostgreSQL sample $($evidence | ConvertTo-Json -Compress -Depth 4)"
    }

    $listener = [Net.Sockets.TcpListener]::new([Net.IPAddress]::Loopback, 0)
    $listener.Start()
    $port = $listener.LocalEndpoint.Port
    try {
        $result = Test-PostgresConnection -Port $port
        Assert-True ($result.status -eq 'connected') 'Probe did not connect to the live listener.'
        $accepted = $listener.AcceptTcpClient()
        $accepted.Dispose()
    } finally { $listener.Stop() }
    # Allow refusal classification to finish before cancellation on Windows.
    # The sampler still uses the default 500 ms observation deadline.
    $result = Test-PostgresConnection -Port $port -TimeoutMilliseconds 4000
    Assert-True ($result.status -eq 'error' -and $result.socket_error -eq 'ConnectionRefused') "Refusal was not preserved: $($result | ConvertTo-Json -Compress)"
    Assert-True ($result.elapsed_ms -lt 5000) 'Refused probe did not complete promptly.'

    Add-Type @'
using System.ComponentModel;
using System.Threading.Tasks;
public static class DiagnosticFailures {
    public static async Task Complete() { await Task.Yield(); }
    public static object Read() { throw new Win32Exception(5, "SECRET_MEMORY_FAILURE"); }
}
'@
    $originalProbe = ${function:Test-PostgresConnection}
    $originalMemory = ${function:Get-WindowsMemoryInfo}
    try {
        # Exercise the actual call boundaries, not direct PowerShell throws.
        ${function:Test-PostgresConnection} = [scriptblock]::Create($originalProbe.ToString().Replace(
            "$" + "client.ConnectAsync('127.0.0.1', `$Port, `$cancel.Token).AsTask()", '[DiagnosticFailures]::Complete()'))
        $result = @(Test-PostgresConnection)
        Assert-True ($result.Count -eq 1 -and $result[0] -is [Collections.IDictionary] -and $result[0].status -eq 'connected') 'Task completion leaked into probe output.'
        ${function:Get-WindowsMemoryInfo} = [scriptblock]::Create($originalMemory.ToString().Replace(
            '[PostgresDiagnostics.Memory]::Read()', '[DiagnosticFailures]::Read()'))
        $sample = Get-PostgresDiagnosticSample $directory
        Assert-True ($sample.memory.error.type -eq 'System.ComponentModel.Win32Exception' -and
            $sample.memory.error.hresult -eq ([ComponentModel.Win32Exception]::new(5)).HResult -and
            $sample.disk.total_bytes -gt 0 -and ($sample | ConvertTo-Json -Depth 8) -notmatch 'SECRET_MEMORY_FAILURE') 'Memory error boundary lost fidelity, privacy or later collection.'
    } finally {
        ${function:Test-PostgresConnection} = $originalProbe
        ${function:Get-WindowsMemoryInfo} = $originalMemory
    }

    & {
        $null = New-Item -ItemType Directory -Path (Join-Path $directory 'data')
        $pidFile = Join-Path $directory 'data/postmaster.pid'
        '100' | Set-Content -LiteralPath $pidFile
        $created = [DateTime]::UtcNow
        $state = @{}
        $fixture = @{
            processes = @(1..40 | ForEach-Object {
                [pscustomobject]@{ ProcessId = $_; ParentProcessId = 999; CreationDate = $created; Name = 'postgres.exe'; WorkingSetSize = 900; PrivatePageCount = 900 }
            }) + @(100..140 | ForEach-Object {
                [pscustomobject]@{ ProcessId = $_; ParentProcessId = $(if ($_ -eq 100) { 99 } else { 100 }); CreationDate = $created; Name = 'postgres.exe'; WorkingSetSize = 10; PrivatePageCount = 20 }
            })
        }
        # There is no listener. Unrelated clusters must not displace the target.
        function Get-CimInstance {
            param($ClassName)
            if ($ClassName -eq 'Win32_Process') { $fixture.processes }
        }
        $sample = Get-PostgresDiagnosticSample $directory $state
        Assert-True ($sample.processes.count -eq 41 -and $sample.processes.details.Count -eq 32 -and $sample.processes.details[0].pid -eq 100 -and $sample.processes.working_set_bytes -eq 410 -and $sample.processes.private_bytes -eq 820) 'Target cluster was displaced or mixed with unrelated processes.'
        Remove-Item -LiteralPath $pidFile
        $fixture.processes = @($fixture.processes | Where-Object { $_.ProcessId -ne 100 })
        $sample = Get-PostgresDiagnosticSample $directory $state
        Assert-True ($sample.processes.count -eq 40 -and $sample.processes.postmaster_pid -eq 100) 'Postmaster exit lost surviving target processes.'
        $fixture.processes += [pscustomobject]@{ ProcessId = 100; ParentProcessId = 99; CreationDate = $created.AddSeconds(1); Name = 'postgres.exe' }
        $sample = Get-PostgresDiagnosticSample $directory $state
        Assert-True ($sample.processes.count -eq 0) 'A reused postmaster PID was attributed to the target.'
    }

    $ticks = (Get-Process -Id $PID).StartTime.ToUniversalTime().Ticks
    $collector = ${function:Get-PostgresDiagnosticSample}
    try {
        function Get-PostgresDiagnosticSample { @{ sampler_pid = $PID } }
        $output = Join-Path $directory 'limits.jsonl'
        Invoke-PostgresSampler $output $PID $ticks $directory -MaxSamples 1 -IntervalMilliseconds 0
        Assert-True (@(Get-Content $output).Count -eq 1) 'Sample limit failed.'
        Invoke-PostgresSampler $output $PID $ticks $directory -MaxBytes 1
        Assert-True ((Get-Item $output).Length -eq 0) 'Byte limit failed.'
        Invoke-PostgresSampler $output $PID ($ticks + 1) $directory
        Assert-True ((Get-Item $output).Length -eq 0) 'Parent identity check failed.'
        Invoke-PostgresSampler $output $PID $ticks $directory -MaxSeconds 0
        Assert-True ((Get-Item $output).Length -eq 0) 'Duration limit failed.'
        foreach ($phase in @('before', 'collection', 'interval')) {
            & {
                $stopFile = Join-Path $directory "$phase.stop"
                if ($phase -eq 'before') { [IO.File]::WriteAllText($stopFile, '') }
                function Get-PostgresDiagnosticSample {
                    if ($phase -eq 'collection') { [IO.File]::WriteAllText($stopFile, '') }
                    @{ sampler_pid = $PID }
                }
                function Start-Sleep { [IO.File]::WriteAllText($stopFile, '') }
                Invoke-PostgresSampler $output $PID $ticks $directory -StopFile $stopFile -MaxSamples 3
                $samples = @(Get-Content $output | ForEach-Object { $_ | ConvertFrom-Json -AsHashtable })
                $expected = if ($phase -eq 'before') { 1 } else { 2 }
                Assert-True ($samples.Count -eq $expected -and $samples[-1].final) "Shutdown during $phase did not flush a final sample."
                Remove-Item -LiteralPath $stopFile
            }
        }
    } finally { ${function:Get-PostgresDiagnosticSample} = $collector }

    # Substitute only launch and make commands to check owned cleanup on failure.
    & {
        $state = @{ killed = $false; disposed = $false; cooperative = $false; waits = @() }
        function Start-Process {
            $child = [pscustomobject]@{ HasExited = $false }
            $child | Add-Member ScriptMethod Kill { $state.killed = $true; $this.HasExited = $true }
            $child | Add-Member ScriptMethod WaitForExit {
                param($milliseconds)
                $state.waits += $milliseconds
                if ($state.cooperative) {
                    Assert-True (@(Get-ChildItem -LiteralPath $directory -Filter '*.stop').Count -eq 1) 'Shutdown was not signaled before waiting.'
                    $this.HasExited = $true
                }
                return $this.HasExited
            }
            $child | Add-Member ScriptMethod Dispose { $state.disposed = $true }
            return $child
        }
        $directModulePath = $env:PSModulePath
        $transport = $env:CODER_TEST_PSMODULEPATH_PRESENT
        $env:CODER_TEST_PSMODULEPATH_PRESENT = $null
        function make {
            Assert-True ($env:PSModulePath -ceq $directModulePath) 'Direct runner changed module path without transport.'
            $global:LASTEXITCODE = 2
        }
        & "$PSScriptRoot/test-with-postgres-diagnostics.ps1" -OutputDirectory $directory -DatabasePath $directory
        Assert-True ($LASTEXITCODE -eq 2 -and $state.killed -and $state.disposed) 'Runner lost make failure or owned cleanup.'
        Assert-True (($state.waits -join ',') -eq '15000,2000') 'Fallback shutdown exceeded its wait bounds.'
        $state.cooperative = $true
        $state.killed = $false
        $state.disposed = $false
        & "$PSScriptRoot/test-with-postgres-diagnostics.ps1" -OutputDirectory $directory -DatabasePath $directory
        Assert-True ($LASTEXITCODE -eq 2 -and -not $state.killed -and $state.disposed) 'Cooperative shutdown killed the sampler or lost make status.'
        function Start-Process { throw 'SECRET_STARTUP_FAILURE' }
        $warnings = & "$PSScriptRoot/test-with-postgres-diagnostics.ps1" -OutputDirectory $directory -DatabasePath $directory 3>&1
        Assert-True ("$warnings" -match 'PostgreSQL sampler did not start') 'Expected startup warning missing.'
        $errors = Get-Content (Join-Path $directory 'sampler-errors.jsonl') -Raw
        Assert-True ($LASTEXITCODE -eq 2 -and $errors -match 'hresult' -and $errors -notmatch 'SECRET_STARTUP_FAILURE') 'Startup failure blocked make or leaked its message.'
        $env:CODER_TEST_PSMODULEPATH_PRESENT = $transport
    }

    # Exercise the action's default-path resolution, including quoted paths.
    & {
        $action = Get-Content "$PSScriptRoot/../actions/test-go-pg/action.yaml" -Raw
        $resolve = [regex]::Match($action, '(?ms)- name: Resolve PostgreSQL path \(Windows\).*?      run: \|\r?\n(?<script>(?:        [^\r\n]*\r?\n)+)').Groups['script'].Value
        Assert-True ($resolve.Length -gt 0) 'Windows database path resolution missing.'
        $savedPath = $env:EMBEDDED_PG_PATH
        $savedOutput = $env:GITHUB_OUTPUT
        try {
            foreach ($path in @('', $directory)) {
                $env:EMBEDDED_PG_PATH = $path
                $env:GITHUB_OUTPUT = Join-Path $directory 'action-output'
                Remove-Item -LiteralPath $env:GITHUB_OUTPUT -Force -ErrorAction SilentlyContinue
                & ([scriptblock]::Create($resolve))
                $expected = if ($path) { $path } else { Join-Path ([IO.Path]::GetTempPath()) 'coder-test-postgres' }
                Assert-True ((Get-Content -LiteralPath $env:GITHUB_OUTPUT) -ceq "path=$expected") 'Action changed the configured or default PostgreSQL path.'
            }
        } finally {
            $env:EMBEDDED_PG_PATH = $savedPath
            $env:GITHUB_OUTPUT = $savedOutput
        }
    }

    # Run the concrete entry point with a real make process. No callback runner.
    @'
ifneq ($(origin PSModulePath),$(EXPECTED_MODULE_ORIGIN))
$(error Module path presence changed)
endif
ifneq ($(PSModulePath),$(CODER_TEST_PSMODULEPATH))
$(error Module path value changed)
endif
test:
	@echo test-output
	@exit 7
test-race:
	@echo race-output
'@ | Set-Content (Join-Path $directory 'Makefile')
    # Use the actual pre-pwsh exports, not a value assigned after shell startup.
    $capture = (Get-Content "$PSScriptRoot/../actions/test-go-pg/action.yaml" |
        Where-Object { $_ -match '^\s+export CODER_TEST_PSMODULEPATH' }) -join "`n"
    Assert-True ($capture.Length -gt 0) 'Caller module-path capture missing.'
    $variables = @('PSModulePath', 'EXPECTED_MODULE_ORIGIN')
    $saved = @{}
    foreach ($name in $variables) { $saved[$name] = [Environment]::GetEnvironmentVariable($name) }
    Push-Location $directory
    try {
        foreach ($target in @('test', 'test-race')) {
            $present = $target -eq 'test'
            $env:PSModulePath = if ($present) { 'caller module path;second-path' } else { $null }
            $env:EXPECTED_MODULE_ORIGIN = if ($present) { 'environment' } else { 'undefined' }
            $runDirectory = Join-Path $directory "run-$target"
            $output = & bash -c ($capture + "`n" + 'exec "$@"') -- ((Get-Process -Id $PID).Path.Replace('\', '/')) -NoProfile -File "$PSScriptRoot/test-with-postgres-diagnostics.ps1" -OutputDirectory $runDirectory -DatabasePath $directory -Target $target 2>&1
            $expected = if ($target -eq 'test') { 2 } else { 0 }
            Assert-True ($LASTEXITCODE -eq $expected -and "$output" -match $(if ($target -eq 'test') { 'test-output' } else { 'race-output' })) "Runner changed make exit/output: $LASTEXITCODE $output"
            Assert-True (-not (Test-Path (Join-Path $runDirectory 'sampler-errors.jsonl'))) 'Production launcher reported sampler errors.'
            $samples = @(Get-Content -LiteralPath (Join-Path $runDirectory 'samples.jsonl') | ForEach-Object { $_ | ConvertFrom-Json -AsHashtable })
            Assert-True ($samples.Count -gt 0 -and $samples[-1].final -and $samples[-1].sampler_pid -gt 0 -and $null -ne $samples[-1].timestamp -and $null -eq $samples[-1].error) 'Production launcher did not flush a valid final sample.'
            Assert-True ($null -eq (Get-Process -Id $samples[-1].sampler_pid -ErrorAction SilentlyContinue)) 'Production sampler survived wrapper exit.'
        }
    } finally {
        Pop-Location
        foreach ($name in $variables) { [Environment]::SetEnvironmentVariable($name, $saved[$name]) }
    }
    Write-Host 'PASS: native smoke when requested, probe, memory boundary, limits and make exits'
} finally {
    Remove-Item -LiteralPath $directory -Recurse -Force
}
exit 0
