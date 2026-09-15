# Run with pwsh -NoProfile -File .github/scripts/postgres-diagnostics.tests.ps1.
# -Native also verifies real Windows collection against the started PostgreSQL.
param([switch] $Native, [string] $NativeDatabasePath, [string] $NativeOutputDirectory)
$ErrorActionPreference = 'Stop'
$PSNativeCommandUseErrorActionPreference = $false
. "$PSScriptRoot/postgres-diagnostics.ps1"
. "$PSScriptRoot/test-with-postgres-diagnostics.ps1"

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
        # Run the real collector before defining any CIM fixtures. A separate
        # owned process bounds the check even if a Windows provider gets stuck.
        $nativeOutput = Join-Path $directory 'native.jsonl'
        $paths = @((Join-Path $PSScriptRoot 'postgres-diagnostics.ps1'), $nativeOutput, $NativeDatabasePath) |
            ForEach-Object { "'" + $_.Replace("'", "''") + "'" }
        $ticks = (Get-Process -Id $PID).StartTime.ToUniversalTime().Ticks
        $command = "`$ErrorActionPreference = 'Stop'; . $($paths[0]); Invoke-PostgresSampler -OutputFile $($paths[1]) -ParentId $PID -ParentStartTicks $ticks -DatabasePath $($paths[2]) -MaxSamples 1 -IntervalMilliseconds 0"
        $encoded = [Convert]::ToBase64String([Text.Encoding]::Unicode.GetBytes($command))
        $nativeSampler = Start-Process -FilePath (Get-Process -Id $PID).Path -ArgumentList @(
            '-NoProfile', '-NonInteractive', '-EncodedCommand', $encoded
        ) -PassThru -NoNewWindow
        try {
            Assert-True ($nativeSampler.WaitForExit(20000)) 'Native sampler did not complete within 20 seconds.'
            Assert-True ($nativeSampler.ExitCode -eq 0) 'Native sampler process failed.'
            $lines = @(Get-Content -LiteralPath $nativeOutput)
            Assert-True ($lines.Count -eq 1) 'Native sampler did not write one completed sample.'
            $nativeSample = $lines[0] | ConvertFrom-Json -AsHashtable
            Assert-NativeSample $nativeSample
            Assert-True ($nativeSample.sampler_pid -eq $nativeSampler.Id) 'Sample came from an unexpected process.'
            # Only these selected fields reach the success log, not a process
            # list or raw errors. This evidence also appears on green cached runs.
            $evidence = @{
                timestamp = $nativeSample.timestamp
                sampler_pid = $nativeSample.sampler_pid
                listener_pids = @($nativeSample.tcp.listeners.pid | Select-Object -First 16)
                available_bytes = $nativeSample.memory.available_bytes
                committed_bytes = $nativeSample.memory.committed_bytes
                commit_limit_bytes = $nativeSample.memory.commit_limit_bytes
                disk_free_bytes = $nativeSample.disk.free_bytes
                disk_total_bytes = $nativeSample.disk.total_bytes
                connect_status = $nativeSample.connect.status
                connect_elapsed_ms = $nativeSample.connect.elapsed_ms
            } | ConvertTo-Json -Compress
            Write-Host "PASS: native Windows PostgreSQL sample $evidence"
        } catch {
            # Retain the real failure sample outside disposable fixtures, where
            # the action's existing diagnostic artifact can collect it.
            try {
                $null = New-Item -ItemType Directory -Path $NativeOutputDirectory -Force -ErrorAction Stop
                Copy-Item -LiteralPath $nativeOutput -Destination (Join-Path $NativeOutputDirectory 'native.jsonl') -ErrorAction Stop
            } catch {
                Write-Warning 'Could not preserve the native PostgreSQL diagnostic sample.'
            }
            throw
        } finally {
            if (-not $nativeSampler.HasExited) { $nativeSampler.Kill(); $null = $nativeSampler.WaitForExit(2000) }
            $nativeSampler.Dispose()
        }
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
    $result = Test-PostgresConnection -Port $port
    Assert-True ($result.status -eq 'error' -and $result.socket_error -eq 'ConnectionRefused') "Refusal was not preserved: $($result | ConvertTo-Json -Compress)"
    Assert-True ($result.elapsed_ms -lt 5000) 'Refused probe did not complete promptly.'

    # Fixtures deliberately include fields that must never be serialized.
    function Get-CimInstance {
        param($Namespace, $ClassName, $Filter, $Property, $OperationTimeoutSec, $ErrorAction)
        switch ($ClassName) {
            'MSFT_NetTCPConnection' {
                [pscustomobject]@{ State = 2; LocalAddress = '127.0.0.1'; OwningProcess = 41 }
                [pscustomobject]@{ State = 5; LocalAddress = '127.0.0.1'; OwningProcess = 41 }
                [pscustomobject]@{ State = 5; LocalAddress = '127.0.0.1'; OwningProcess = 41 }
            }
            'Win32_Process' {
                foreach ($id in 41..80) {
                    [pscustomobject]@{
                        ProcessId = $id; ParentProcessId = 40; CreationDate = [DateTime]::UtcNow
                        Name = 'postgres.exe'; WorkingSetSize = 100; PrivatePageCount = 200
                        CommandLine = 'password=DO_NOT_LOG'; Environment = 'SECRET_ENV'
                    }
                }
            }
            'Win32_PerfFormattedData_PerfOS_Memory' {
                [pscustomobject]@{ AvailableBytes = 11; CommittedBytes = 22; CommitLimit = 33 }
            }
        }
    }
    $sample = Get-PostgresDiagnosticSample -DatabasePath $directory
    Assert-True ($sample.tcp.listeners[0].pid -eq 41) 'Listener owner missing.'
    Assert-True ([DateTime]::Parse($sample.timestamp).ToUniversalTime() -gt [DateTime]::UtcNow.AddMinutes(-1)) 'Sample timestamp missing or stale.'
    Assert-True ([DateTime]::Parse($sample.processes.details[0].created).ToUniversalTime() -gt [DateTime]::UtcNow.AddMinutes(-1)) 'Process creation time missing or stale.'
    Assert-True ($sample.tcp.state_counts['5'] -eq 2) 'TCP states were not aggregated.'
    Assert-True ($sample.processes.count -eq 40 -and $sample.processes.details.Count -eq 32) 'Process output was not bounded.'
    Assert-True ($sample.processes.working_set_bytes -eq 4000) 'Process totals excluded truncated details.'
    Assert-True ($sample.processes.details[0].parent_pid -eq 40) 'Process parent missing.'
    Assert-True ($sample.memory.committed_bytes -eq 22 -and $sample.memory.commit_limit_bytes -eq 33) 'Commit pressure missing.'
    Assert-True ($sample.disk.total_bytes -gt 0) 'Disk capacity missing.'
    # Exercise the native verifier with fixtures too, without presenting this
    # as real Windows CIM validation. Missing fail-open data must fail the check.
    $sample.connect = @{ status = 'connected'; elapsed_ms = 1 }
    $validSample = $sample | ConvertTo-Json -Depth 8
    Assert-NativeSample ($validSample | ConvertFrom-Json -AsHashtable)
    foreach ($field in @('tcp', 'processes', 'memory', 'disk', 'connect')) {
        $broken = $validSample | ConvertFrom-Json -AsHashtable
        $broken[$field] = @{ error = @{ type = 'FixtureError'; hresult = 1 } }
        $rejected = $false
        try { Assert-NativeSample $broken } catch { $rejected = $true }
        Assert-True $rejected "Native verifier accepted missing $field data."
        if ($field -eq 'memory') {
            # Exercise the actual native control flow with only its host/process
            # boundary mocked. The failing real-sample path must retain evidence.
            $ast = [Management.Automation.Language.Parser]::ParseFile($PSCommandPath, [ref]$null, [ref]$null)
            $nativeBlock = $ast.Find({ param($node)
                $node -is [Management.Automation.Language.IfStatementAst] -and $node.Clauses[0].Item1.Extent.Text -eq '$Native'
            }, $true).Extent.Text.Replace('$PSScriptRoot', "'" + $PSScriptRoot.Replace("'", "''") + "'")
            foreach ($preservationFails in @($false, $true)) {
                & {
                    $Native = $true
                    $NativeDatabasePath = $directory
                    $NativeOutputDirectory = Join-Path $directory 'native-artifact'
                    $state = @{ disposed = $false }
                    function Assert-True {
                        param([bool] $Condition, [string] $Message)
                        if ($Message -ne 'Native CIM validation requires Windows.' -and -not $Condition) { throw $Message }
                    }
                    function Start-Process {
                        param($FilePath, $ArgumentList, [switch] $PassThru, [switch] $NoNewWindow)
                        $broken | ConvertTo-Json -Depth 8 -Compress | Set-Content -LiteralPath $nativeOutput
                        $process = [pscustomobject]@{ HasExited = $true; ExitCode = 0; Id = 123 }
                        $process | Add-Member ScriptMethod WaitForExit { param($milliseconds) return $true }
                        $process | Add-Member ScriptMethod Dispose { $state.disposed = $true }
                        return $process
                    }
                    if ($preservationFails) {
                        function Copy-Item { throw 'SECRET_PRESERVATION_FAILURE' }
                    }
                    $caught = $null
                    try { . ([scriptblock]::Create($nativeBlock)) } catch { $caught = $_.Exception.Message }
                    Assert-True ($caught -eq 'Native resource field available_bytes missing or nonnumeric.') 'Preservation replaced the native assertion.'
                    Assert-True $state.disposed 'Native failure skipped owned process disposal.'
                    if (-not $preservationFails) {
                        $saved = Get-Content -LiteralPath (Join-Path $NativeOutputDirectory 'native.jsonl') -Raw | ConvertFrom-Json
                        Assert-True ($saved.memory.error.type -eq 'FixtureError' -and $saved.tcp.listeners[0].pid -eq 41) 'Native failure sample was not preserved.'
                    }
                }
            }
        }
    }
    $broken = $validSample | ConvertFrom-Json -AsHashtable
    $broken.connect.elapsed_ms = 5000
    $rejected = $false
    try { Assert-NativeSample $broken } catch { $rejected = $true }
    Assert-True $rejected 'Native verifier accepted an over-budget probe.'
    $json = $sample | ConvertTo-Json -Depth 8
    Assert-True ($json -notmatch 'DO_NOT_LOG|SECRET_ENV|CommandLine|Environment') 'Sensitive process fields leaked.'
    $failure = Invoke-DiagnosticQuery { throw 'password=DO_NOT_LOG' }
    Assert-True ($null -ne $failure.error -and ($failure | ConvertTo-Json) -notmatch 'DO_NOT_LOG') 'Collector error was lost or leaked its message.'

    # Inject constructor failure into the actual probe body, not a duplicate
    # implementation or a production-only test seam. Later collectors must run.
    $originalProbe = ${function:Test-PostgresConnection}
    try {
        foreach ($allocation in @('[Net.Sockets.TcpClient]::new()', '[Threading.CancellationTokenSource]::new($TimeoutMilliseconds)')) {
            $failureExpression = if ($allocation -like '*TcpClient*') {
                '$(throw [Net.Sockets.SocketException]::new(10024))'
            } else { '$(throw [InvalidOperationException]::new("SECRET_ALLOCATION_FAILURE"))' }
            ${function:Test-PostgresConnection} = [scriptblock]::Create($originalProbe.ToString().Replace($allocation, $failureExpression))
            $allocationSample = Get-PostgresDiagnosticSample -DatabasePath $directory
            Assert-True ($allocationSample.connect.status -eq 'error' -and
                ($null -ne $allocationSample.connect.socket_error -or $null -ne $allocationSample.connect.error.type)) 'Probe allocation failure escaped observation.'
            Assert-True (($allocationSample | ConvertTo-Json -Depth 8) -notmatch 'SECRET_ALLOCATION_FAILURE') 'Allocation failure leaked its message.'
            Assert-True ($allocationSample.tcp.listeners[0].pid -eq 41 -and $allocationSample.processes.count -eq 40 -and
                $allocationSample.memory.committed_bytes -eq 22 -and $allocationSample.disk.total_bytes -gt 0) 'Probe allocation failure lost other collectors.'
        }
    } finally { ${function:Test-PostgresConnection} = $originalProbe }

    $parent = Get-Process -Id $PID
    $ticks = $parent.StartTime.ToUniversalTime().Ticks
    Assert-True (Test-DiagnosticParent $PID $ticks) 'Live parent identity rejected.'
    Assert-True (-not (Test-DiagnosticParent $PID ($ticks + 1))) 'Reused PID identity was accepted.'
    $originalCollector = ${function:Get-PostgresDiagnosticSample}
    function Get-PostgresDiagnosticSample { @{ timestamp = 'fixture'; value = 1 } }
    $output = Join-Path $directory 'bounded.jsonl'
    Invoke-PostgresSampler $output $PID $ticks $directory -IntervalMilliseconds 0 -MaxSamples 2
    Assert-True (@(Get-Content $output).Count -eq 2) 'Sampler did not honor its record bound.'
    $oneRecordBytes = (Get-Item $output).Length / 2
    Invoke-PostgresSampler $output $PID $ticks $directory -IntervalMilliseconds 0 -MaxBytes $oneRecordBytes
    Assert-True ((Get-Item $output).Length -eq $oneRecordBytes) 'Sampler byte budget does not match serialized bytes.'
    Invoke-PostgresSampler $output $PID $ticks $directory -IntervalMilliseconds 0 -MaxBytes 1
    Assert-True ((Get-Item $output).Length -eq 0) 'Sampler exceeded its byte budget.'
    Invoke-PostgresSampler $output $PID ($ticks + 1) $directory -IntervalMilliseconds 0
    Assert-True ((Get-Item $output).Length -eq 0) 'Sampler collected after parent identity changed.'
    Invoke-PostgresSampler $output $PID $ticks $directory -MaxSeconds 0
    Assert-True ((Get-Item $output).Length -eq 0) 'Sampler ignored its deadline.'
    function Get-PostgresDiagnosticSample { throw 'SECRET_COLLECTION_FAILURE' }
    Invoke-PostgresSampler $output $PID $ticks $directory -IntervalMilliseconds 0 -MaxSamples 1
    $record = Get-Content $output -Raw
    Assert-True ($record -match 'error' -and $record -notmatch 'SECRET_COLLECTION_FAILURE') 'Sampler did not preserve a sanitized collection failure.'
    ${function:Get-PostgresDiagnosticSample} = $originalCollector

    # A pipe gives an explicit readiness barrier. The process stays blocked in a
    # read while the test callback completes or throws, exercising owned cleanup.
    $fixture = Join-Path $directory 'sampler.ps1'
    @'
param($OutputFile, $ParentId, $ParentStartTicks, $DatabasePath)
$pipe = [IO.Pipes.NamedPipeClientStream]::new('.', $DatabasePath, [IO.Pipes.PipeDirection]::InOut)
$pipe.Connect(15000)
$writer = [IO.StreamWriter]::new($pipe)
$writer.AutoFlush = $true
$writer.WriteLine($PID)
$null = $pipe.ReadByte()
'@ | Set-Content $fixture
    foreach ($fail in @($false, $true)) {
        $pipeName = [Guid]::NewGuid().ToString()
        $pipe = [IO.Pipes.NamedPipeServerStream]::new($pipeName, [IO.Pipes.PipeDirection]::InOut, 1,
            [IO.Pipes.PipeTransmissionMode]::Byte, [IO.Pipes.PipeOptions]::Asynchronous)
        $state = @{ child = 0; ran = $false }
        try {
            $caught = $false
            try {
                Invoke-TestsWithPostgresDiagnostics -OutputDirectory $directory -DatabasePath $pipeName -SamplerPath $fixture -RunTests {
                    $wait = $pipe.WaitForConnectionAsync()
                    Assert-True ($wait.Wait(15000)) 'Sampler never reached readiness.'
                    $reader = [IO.StreamReader]::new($pipe)
                    $state.child = [int]$reader.ReadLine()
                    $state.ran = $true
                    Assert-True ($null -ne (Get-Process -Id $state.child)) 'Sampler was not alive during tests.'
                    if ($fail) { throw 'TEST_FAILURE' }
                }
            } catch {
                if ($_.Exception.Message -ne 'TEST_FAILURE') { throw }
                $caught = $true
            }
            Assert-True ($state.ran -and $caught -eq $fail) 'Wrapper changed test failure behavior.'
            Assert-True ($null -eq (Get-Process -Id $state.child -ErrorAction SilentlyContinue)) 'Wrapper left its sampler process alive.'
        } finally { $pipe.Dispose() }
    }

    # Kill a real wrapper without allowing its finally block to run. The
    # sampler must stop itself on parent loss, not rely on runner tree cleanup.
    $orphanFixture = Join-Path $directory 'parent-loss-sampler.ps1'
    @'
param($OutputFile, $ParentId, $ParentStartTicks, $DatabasePath)
. $DatabasePath -OutputFile $OutputFile -ParentId $ParentId -ParentStartTicks $ParentStartTicks -DatabasePath $DatabasePath
function Get-PostgresDiagnosticSample { @{ sampler_pid = $PID } }
$pipe = [IO.Pipes.NamedPipeClientStream]::new('.', [IO.Path]::GetFileName([IO.Path]::GetDirectoryName($OutputFile)), [IO.Pipes.PipeDirection]::Out)
$pipe.Connect(15000)
$writer = [IO.StreamWriter]::new($pipe)
$writer.AutoFlush = $true
$writer.WriteLine($PID)
Invoke-PostgresSampler $OutputFile $ParentId $ParentStartTicks $DatabasePath -IntervalMilliseconds 25
$writer.Dispose()
'@ | Set-Content $orphanFixture
    $pipeName = [Guid]::NewGuid().ToString()
    $pipe = [IO.Pipes.NamedPipeServerStream]::new($pipeName, [IO.Pipes.PipeDirection]::In, 1,
        [IO.Pipes.PipeTransmissionMode]::Byte, [IO.Pipes.PipeOptions]::Asynchronous)
    $literalPaths = @((Join-Path $PSScriptRoot 'test-with-postgres-diagnostics.ps1'),
        (Join-Path $directory $pipeName), (Join-Path $PSScriptRoot 'postgres-diagnostics.ps1'), $orphanFixture) |
        ForEach-Object { "'" + $_.Replace("'", "''") + "'" }
    $command = ". $($literalPaths[0]); Invoke-TestsWithPostgresDiagnostics -OutputDirectory $($literalPaths[1]) -DatabasePath $($literalPaths[2]) -SamplerPath $($literalPaths[3]) -RunTests { [Threading.ManualResetEventSlim]::new().Wait() }"
    $encoded = [Convert]::ToBase64String([Text.Encoding]::Unicode.GetBytes($command))
    $orphan = $null
    $owner = Start-Process -FilePath (Get-Process -Id $PID).Path -ArgumentList @('-NoProfile', '-EncodedCommand', $encoded) -PassThru -NoNewWindow
    try {
        Assert-True ($pipe.WaitForConnectionAsync().Wait(15000)) 'Parent-loss sampler did not start.'
        $reader = [IO.StreamReader]::new($pipe)
        $orphan = Get-Process -Id ([int]$reader.ReadLine())
        $owner.Kill()
        Assert-True ($owner.WaitForExit(2000)) 'Fixture wrapper did not exit.'
        Assert-True ($orphan.WaitForExit(15000)) 'Sampler survived abrupt parent loss.'
    } finally {
        foreach ($owned in @($owner, $orphan)) {
            if ($null -ne $owned) {
                if (-not $owned.HasExited) { $owned.Kill(); $null = $owned.WaitForExit(2000) }
                $owned.Dispose()
            }
        }
        $pipe.Dispose()
    }

    $ran = @{ value = $false }
    function Start-Process { throw 'SECRET_STARTUP_FAILURE' }
    Invoke-TestsWithPostgresDiagnostics -OutputDirectory $directory -DatabasePath $directory `
        -SamplerPath (Join-Path $directory 'missing.ps1') -RunTests { $ran.value = $true }
    Remove-Item function:Start-Process
    Assert-True $ran.value 'Sampler startup failure prevented tests.'
    Assert-True ((Get-Item (Join-Path $directory 'sampler-errors.jsonl')).Length -gt 0) 'Sampler startup error was not saved.'

    $backgroundErrors = Join-Path $directory 'sampler-errors.jsonl'
    Remove-Item -LiteralPath $backgroundErrors
    $backgroundStarted = [DateTime]::UtcNow
    $badFixture = Join-Path $directory 'bad-sampler.ps1'
    $pipeName = [Guid]::NewGuid().ToString()
    $pipe = [IO.Pipes.NamedPipeServerStream]::new($pipeName, [IO.Pipes.PipeDirection]::InOut, 1,
        [IO.Pipes.PipeTransmissionMode]::Byte, [IO.Pipes.PipeOptions]::Asynchronous)
    @'
param($OutputFile, $ParentId, $ParentStartTicks, $DatabasePath)
$pipe = [IO.Pipes.NamedPipeClientStream]::new('.', $DatabasePath, [IO.Pipes.PipeDirection]::InOut)
$pipe.Connect(15000)
$pipe.Dispose()
throw 'SECRET_BACKGROUND_FAILURE'
'@ | Set-Content $badFixture
    try {
        Invoke-TestsWithPostgresDiagnostics -OutputDirectory $directory -DatabasePath $pipeName -SamplerPath $badFixture -RunTests {
            Assert-True ($pipe.WaitForConnectionAsync().Wait(15000)) 'Failing sampler did not start.'
            # EOF proves the fixture ran, then wait for the owned process to
            # finish writing its error without polling or sleeping.
            $null = $pipe.ReadByte()
            Assert-True ($sampler.WaitForExit(15000)) 'Failing sampler did not exit within its deadline.'
        }
    } finally { $pipe.Dispose() }
    $errors = @(Get-Content -LiteralPath $backgroundErrors)
    Assert-True ($errors.Count -eq 1) 'Expected one fresh background error record.'
    $errorRecord = $errors[0] | ConvertFrom-Json
    Assert-True ($errorRecord.type -eq 'System.Management.Automation.RuntimeException' -and
        $null -ne $errorRecord.hresult -and ([DateTime]$errorRecord.timestamp).ToUniversalTime() -ge $backgroundStarted -and
        $errors[0] -notmatch 'SECRET_BACKGROUND_FAILURE') 'Background error missing, stale or unsanitized.'

    # Exercise the public wrapper entry point and real native exit propagation.
    @'
test:
	@echo test-output
	@exit 7
test-race:
	@echo race-output
'@ | Set-Content (Join-Path $directory 'Makefile')
    Push-Location $directory
    try {
        $wrapper = Join-Path $PSScriptRoot 'test-with-postgres-diagnostics.ps1'
        $pwsh = (Get-Process -Id $PID).Path
        $nativeOutput = & $pwsh -NoProfile -File $wrapper -OutputDirectory $directory -DatabasePath $directory -Target test 2>&1
        Assert-True ($LASTEXITCODE -eq 2 -and "$nativeOutput" -match 'test-output') 'Failed make exit/output changed.'
        $nativeOutput = & $pwsh -NoProfile -File $wrapper -OutputDirectory $directory -DatabasePath $directory -Target test-race 2>&1
        Assert-True ($LASTEXITCODE -eq 0 -and "$nativeOutput" -match 'race-output') 'Successful make target/output changed.'
    } finally { Pop-Location }
    # Execute the actual composite action's bash block, rather than a copy of
    # its routing logic, with both OS paths and both make targets.
    $action = Get-Content (Join-Path $PSScriptRoot '../actions/test-go-pg/action.yaml') -Raw
    $verification = [regex]::Match($action, '(?ms)^    - name: Verify PostgreSQL diagnostics \(Windows\)\r?\n(.*?)(?=^    - name:)').Groups[1].Value
    Assert-True ($verification.Contains("if: runner.os == 'Windows'") -and $verification.Contains('postgres-diagnostics.tests.ps1 -Native -NativeDatabasePath')) 'Action does not invoke native validation on Windows.'
    Assert-True ($verification.Contains('-NativeOutputDirectory "$env:RUNNER_TEMP/postgres-diagnostics"')) 'Native failure evidence does not target the diagnostic artifact.'
    $testStep = [regex]::Match($action, '(?ms)^    - name: Run tests\r?\n(.*?)(?=^    - name:)').Groups[1].Value
    $run = [regex]::Match($testStep, '(?ms)^      run: \|\r?\n(.*)').Groups[1].Value
    Assert-True ($run.Length -gt 0) 'Could not extract the test action command.'
    $run = [regex]::Replace($run, '(?m)^        ', '')
    $actionScript = Join-Path $directory 'run-tests.sh'
    [IO.File]::WriteAllText($actionScript, $run.Replace("`r`n", "`n"))
    $scripts = Join-Path $directory '.github/scripts'
    $null = New-Item -ItemType Directory -Path $scripts -Force
    Copy-Item "$PSScriptRoot/postgres-diagnostics.ps1", "$PSScriptRoot/test-with-postgres-diagnostics.ps1" $scripts
    $variables = @('RUNNER_OS', 'RUNNER_TEMP', 'EMBEDDED_PG_PATH', 'RACE_DETECTION', 'GOTESTSUM_JSONFILE_INPUT', 'PATH')
    $saved = @{}
    foreach ($name in $variables) { $saved[$name] = [Environment]::GetEnvironmentVariable($name) }
    Push-Location $directory
    try {
        $env:PATH = "$PSHOME$([IO.Path]::PathSeparator)$env:PATH"
        $env:RUNNER_TEMP = $directory
        $env:EMBEDDED_PG_PATH = $directory
        $env:GOTESTSUM_JSONFILE_INPUT = 'default'
        foreach ($os in @('Windows', 'Linux')) {
            foreach ($race in @('true', 'false')) {
                $env:RUNNER_OS = $os
                $env:RACE_DETECTION = $race
                $actionOutput = & bash ($actionScript.Replace('\', '/')) 2>&1
                $expected = if ($race -eq 'true') { 0 } else { 2 }
                Assert-True ($LASTEXITCODE -eq $expected) "Action changed $os/$race test exit."
                $expectedOutput = if ($race -eq 'true') { 'race-output' } else { 'test-output' }
                Assert-True ("$actionOutput" -match $expectedOutput) "Action ran the wrong target for $os/$race."
            }
        }
        $artifact = [regex]::Match($action, '(?ms)^    - name: Upload PostgreSQL diagnostics \(Windows\)\r?\n(.*?)(?=^    - name:)').Groups[1].Value
        $artifactPath = [regex]::Match($artifact, '(?m)^        path: (.+)').Groups[1].Value.Trim()
        $artifactPath = $artifactPath.Replace('${{ runner.temp }}', $directory)
        # The failure artifact must resolve to the actual files produced by the
        # routed wrapper, including its diagnostic errors.
        Assert-True (Test-Path $artifactPath) 'Artifact does not resolve to the sampler output directory.'
        Assert-True ($artifact -match 'always\(\)' -and $artifact -match '!success\(\)') 'Artifact excludes failure or cancellation.'
    } finally {
        Pop-Location
        foreach ($name in $variables) { [Environment]::SetEnvironmentVariable($name, $saved[$name]) }
    }
    Write-Host 'PASS: probe, collection, limits, parent identity, lifecycle, errors, native exits and action routing'
} finally {
    Remove-Item -LiteralPath $directory -Recurse -Force
}
# Expected failing native commands above must not become the Actions step exit.
exit 0
