param(
    [string] $OutputFile,
    [int] $ParentId,
    [long] $ParentStartTicks,
    [string] $DatabasePath,
    [string] $StopFile
)

# Only selected numeric fields and process identities leave the runner. In
# particular, CIM error messages, command lines and environment are not logged.
function Get-DiagnosticError {
    param($Exception)
    return @{ type = $Exception.GetType().FullName; hresult = $Exception.HResult }
}

function Invoke-DiagnosticQuery {
    param([scriptblock] $Query)
    try {
        & $Query
    } catch {
        @{ error = Get-DiagnosticError $_.Exception }
    }
}

function Test-PostgresConnection {
    param([int] $Port = 5432, [int] $TimeoutMilliseconds = 500)
    $timer = [Diagnostics.Stopwatch]::StartNew()
    $client = $null
    $cancel = $null
    try {
        $client = [Net.Sockets.TcpClient]::new()
        $cancel = [Threading.CancellationTokenSource]::new($TimeoutMilliseconds)
        $null = $client.ConnectAsync('127.0.0.1', $Port, $cancel.Token).AsTask().GetAwaiter().GetResult()
        return @{ status = 'connected'; elapsed_ms = $timer.ElapsedMilliseconds }
    } catch {
        $cause = $_.Exception.GetBaseException()
        $status = if ($null -ne $cancel -and $cancel.IsCancellationRequested) { 'timeout' } else { 'error' }
        $result = @{ status = $status; elapsed_ms = $timer.ElapsedMilliseconds }
        if ($cause -is [Net.Sockets.SocketException]) {
            $result.socket_error = $cause.SocketErrorCode.ToString()
            $result.native_error = $cause.NativeErrorCode
        } else {
            $result.error = Get-DiagnosticError $cause
        }
        return $result
    } finally {
        if ($null -ne $client) { $client.Dispose() }
        if ($null -ne $cancel) { $cancel.Dispose() }
    }
}

function Get-WindowsMemoryInfo {
    # Read system memory directly, without the performance-counter CIM provider.
    # Memory counts are pointer-sized pages; PageSize is measured in bytes.
    if (-not ('PostgresDiagnostics.Memory' -as [type])) {
        Add-Type -TypeDefinition @'
using System;
using System.ComponentModel;
using System.Runtime.InteropServices;
namespace PostgresDiagnostics {
    [StructLayout(LayoutKind.Sequential)]
    public struct PerformanceInformation {
        public uint cb;
        public UIntPtr CommitTotal, CommitLimit, CommitPeak;
        public UIntPtr PhysicalTotal, PhysicalAvailable, SystemCache;
        public UIntPtr KernelTotal, KernelPaged, KernelNonpaged, PageSize;
        public uint HandleCount, ProcessCount, ThreadCount;
    }
    public static class Memory {
        [DllImport("psapi.dll", SetLastError = true)]
        [return: MarshalAs(UnmanagedType.Bool)]
        private static extern bool GetPerformanceInfo(out PerformanceInformation info, uint size);
        public static PerformanceInformation Read() {
            PerformanceInformation info;
            if (!GetPerformanceInfo(out info, (uint)Marshal.SizeOf<PerformanceInformation>())) {
                throw new Win32Exception(Marshal.GetLastWin32Error());
            }
            return info;
        }
    }
}
'@ -ErrorAction Stop
    }
    try {
        return [PostgresDiagnostics.Memory]::Read()
    } catch {
        throw $_.Exception.GetBaseException()
    }
}

function Get-PostgresDiagnosticSample {
    param([string] $DatabasePath, [hashtable] $TargetState = @{})
    $sample = [ordered]@{
        timestamp = [DateTime]::UtcNow.ToString('o')
        sampler_pid = $PID
    }
    $timer = [Diagnostics.Stopwatch]::StartNew()
    # Query before the probe so the probe's own socket is not counted here.
    $sample.tcp = Invoke-DiagnosticQuery {
        $connections = @(Get-CimInstance -Namespace root/StandardCimv2 -ClassName MSFT_NetTCPConnection `
            -Filter 'LocalPort = 5432' -Property LocalAddress, State, OwningProcess `
            -OperationTimeoutSec 2 -ErrorAction Stop)
        $counts = @{}
        foreach ($connection in $connections) {
            $key = [string]$connection.State
            if (-not $counts.ContainsKey($key)) { $counts[$key] = 0 }
            $counts[$key]++
        }
        # MSFT_NetTCPConnection state 2 is Listen. Include wildcard listeners
        # because either can own the loopback endpoint.
        $listeners = @($connections | Where-Object {
            $_.State -eq 2 -and $_.LocalAddress -in @('127.0.0.1', '0.0.0.0', '::', '::1')
        } | Select-Object -First 16 | ForEach-Object {
            @{ address = $_.LocalAddress; pid = [int]$_.OwningProcess }
        })
        @{ listeners = $listeners; state_counts = $counts }
    }
    $sample.connect = Test-PostgresConnection
    $sample.processes = Invoke-DiagnosticQuery {
        # Keep the cluster identity after its listener or PID file disappears.
        # Other tests start unrelated PostgreSQL instances on this runner.
        if (-not $TargetState.ContainsKey('pid')) {
            $TargetState.pid = [int](Get-Content -LiteralPath (Join-Path $DatabasePath 'data/postmaster.pid') -TotalCount 1 -ErrorAction Stop)
        }
        $candidates = @(Get-CimInstance -ClassName Win32_Process -Filter "Name = 'postgres.exe'" `
            -Property ProcessId, ParentProcessId, CreationDate, Name, WorkingSetSize, PrivatePageCount `
            -OperationTimeoutSec 2 -ErrorAction Stop)
        $postmaster = $candidates | Where-Object { $_.ProcessId -eq $TargetState.pid } | Select-Object -First 1
        if ($null -ne $postmaster -and -not $TargetState.ContainsKey('created')) {
            $TargetState.created = $postmaster.CreationDate
        }
        # PostgreSQL backends are direct children of the postmaster. Reject a
        # reused PID, but retain surviving children if the postmaster exits.
        $samePostmaster = $null -eq $postmaster -or $postmaster.CreationDate -eq $TargetState.created
        $processes = @($candidates | Where-Object {
            $samePostmaster -and $null -ne $TargetState.created -and
            (($_.ProcessId -eq $TargetState.pid -and $_.CreationDate -eq $TargetState.created) -or
             ($_.ParentProcessId -eq $TargetState.pid -and $_.CreationDate -ge $TargetState.created))
        })
        $working = ($processes | Measure-Object WorkingSetSize -Sum).Sum
        $private = ($processes | Measure-Object PrivatePageCount -Sum).Sum
        # Bound per-backend output, but retain totals and the postmaster.
        $details = @($processes | Sort-Object { $_.ProcessId -ne $TargetState.pid } |
            Select-Object -First 32 | ForEach-Object {
                @{
                    pid = [int]$_.ProcessId
                    parent_pid = [int]$_.ParentProcessId
                    created = $_.CreationDate.ToUniversalTime().ToString('o')
                    name = $_.Name
                    working_set_bytes = [long]$_.WorkingSetSize
                    private_bytes = [long]$_.PrivatePageCount
                }
            })
        @{ postmaster_pid = $TargetState.pid; count = $processes.Count; working_set_bytes = $working; private_bytes = $private; details = $details }
    }
    $sample.memory = Invoke-DiagnosticQuery {
        $memory = Get-WindowsMemoryInfo
        $pageSize = [long]$memory.PageSize.ToUInt64()
        @{
            available_bytes = [long]([long]$memory.PhysicalAvailable.ToUInt64() * $pageSize)
            committed_bytes = [long]([long]$memory.CommitTotal.ToUInt64() * $pageSize)
            commit_limit_bytes = [long]([long]$memory.CommitLimit.ToUInt64() * $pageSize)
        }
    }
    $sample.disk = Invoke-DiagnosticQuery {
        $root = [IO.Path]::GetPathRoot([IO.Path]::GetFullPath($DatabasePath))
        $drive = [IO.DriveInfo]::new($root)
        @{ free_bytes = $drive.AvailableFreeSpace; total_bytes = $drive.TotalSize }
    }
    $sample.elapsed_ms = $timer.ElapsedMilliseconds
    return $sample
}

function Test-DiagnosticParent {
    param([int] $ParentId, [long] $ParentStartTicks)
    try {
        $parent = Get-Process -Id $ParentId -ErrorAction Stop
        return $parent.StartTime.ToUniversalTime().Ticks -eq $ParentStartTicks
    } catch {
        return $false
    }
}

function Invoke-PostgresSampler {
    param(
        [string] $OutputFile, [int] $ParentId, [long] $ParentStartTicks, [string] $DatabasePath,
        [int] $IntervalMilliseconds = 5000, [int] $MaxSamples = 360,
        [int] $MaxBytes = 8388608, [int] $MaxSeconds = 1800,
        [string] $StopFile
    )
    $writer = [IO.StreamWriter]::new($OutputFile, $false, [Text.UTF8Encoding]::new($false))
    $writer.AutoFlush = $true
    # Use one newline byte on Windows too, matching the output budget.
    $writer.NewLine = "`n"
    $timer = [Diagnostics.Stopwatch]::StartNew()
    $bytes = 0
    $targetState = @{}
    try {
        for ($i = 0; $i -lt $MaxSamples -and $timer.Elapsed.TotalSeconds -lt $MaxSeconds; $i++) {
            if (-not (Test-DiagnosticParent $ParentId $ParentStartTicks)) { break }
            $stopping = $StopFile -and [IO.File]::Exists($StopFile)
            try {
                $record = Get-PostgresDiagnosticSample $DatabasePath $targetState
            } catch {
                $record = @{ timestamp = [DateTime]::UtcNow.ToString('o'); error = Get-DiagnosticError $_.Exception }
            }
            $record.final = [bool]$stopping
            $line = ConvertTo-Json -InputObject $record -Compress -Depth 8
            $bytes += [Text.Encoding]::UTF8.GetByteCount($line) + 1
            if ($bytes -gt $MaxBytes) { break }
            $writer.WriteLine($line)
            if ($stopping) { break }
            # This is the sampling cadence, not a connection or test retry.
            $interval = [Diagnostics.Stopwatch]::StartNew()
            while ($interval.ElapsedMilliseconds -lt $IntervalMilliseconds) {
                if ($StopFile -and [IO.File]::Exists($StopFile)) { break }
                Start-Sleep -Milliseconds ([Math]::Max(0, [Math]::Min(100, $IntervalMilliseconds - $interval.ElapsedMilliseconds)))
            }
        }
    } finally {
        $writer.Dispose()
    }
}

if ($MyInvocation.InvocationName -ne '.') {
    Invoke-PostgresSampler -OutputFile $OutputFile -ParentId $ParentId `
        -ParentStartTicks $ParentStartTicks -DatabasePath $DatabasePath -StopFile $StopFile
}
