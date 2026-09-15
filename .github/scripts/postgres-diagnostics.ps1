param(
    [string] $OutputFile,
    [int] $ParentId,
    [long] $ParentStartTicks,
    [string] $DatabasePath
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
    param([string] $DatabasePath)
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
        $owners = @($sample.tcp.listeners | ForEach-Object { [int]$_.pid })
        $filter = "Name = 'postgres.exe'"
        foreach ($owner in ($owners | Select-Object -Unique)) {
            $filter += " OR ProcessId = $owner"
        }
        $processes = @(Get-CimInstance -ClassName Win32_Process -Filter $filter `
            -Property ProcessId, ParentProcessId, CreationDate, Name, WorkingSetSize, PrivatePageCount `
            -OperationTimeoutSec 2 -ErrorAction Stop)
        $working = ($processes | Measure-Object WorkingSetSize -Sum).Sum
        $private = ($processes | Measure-Object PrivatePageCount -Sum).Sum
        # Bound per-backend output, but retain totals and prioritize listeners.
        $details = @($processes | Sort-Object { $_.ProcessId -notin $owners } |
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
        @{ count = $processes.Count; working_set_bytes = $working; private_bytes = $private; details = $details }
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
        [int] $MaxBytes = 8388608, [int] $MaxSeconds = 1800
    )
    $writer = [IO.StreamWriter]::new($OutputFile, $false, [Text.UTF8Encoding]::new($false))
    $writer.AutoFlush = $true
    # Use one newline byte on Windows too, matching the output budget.
    $writer.NewLine = "`n"
    $timer = [Diagnostics.Stopwatch]::StartNew()
    $bytes = 0
    try {
        for ($i = 0; $i -lt $MaxSamples -and $timer.Elapsed.TotalSeconds -lt $MaxSeconds; $i++) {
            if (-not (Test-DiagnosticParent $ParentId $ParentStartTicks)) { break }
            try {
                $record = Get-PostgresDiagnosticSample $DatabasePath
            } catch {
                $record = @{ timestamp = [DateTime]::UtcNow.ToString('o'); error = Get-DiagnosticError $_.Exception }
            }
            $line = ConvertTo-Json -InputObject $record -Compress -Depth 8
            $bytes += [Text.Encoding]::UTF8.GetByteCount($line) + 1
            if ($bytes -gt $MaxBytes) { break }
            $writer.WriteLine($line)
            # This is the sampling cadence, not a connection or test retry.
            Start-Sleep -Milliseconds $IntervalMilliseconds
        }
    } finally {
        $writer.Dispose()
    }
}

if ($MyInvocation.InvocationName -ne '.') {
    Invoke-PostgresSampler -OutputFile $OutputFile -ParentId $ParentId `
        -ParentStartTicks $ParentStartTicks -DatabasePath $DatabasePath
}
