<#
.SYNOPSIS
    Coder CLI install script for Windows.

.DESCRIPTION
    Installs the Coder CLI from this Coder deployment.
    A matching version of the CLI will be downloaded from this Coder deployment.

.PARAMETER DryRun
    Echo the commands for the install process without running them.

.PARAMETER InstallDir
    The directory to install the Coder CLI to. Defaults to $HOME\.coder\bin
    The installer will add this directory to the user's PATH.

.PARAMETER BinaryName
    The name for the CLI binary. Defaults to "coder"
    Note: in-product documentation will always refer to the CLI as "coder"

.EXAMPLE
    # Install via direct download:
    irm {{ .Origin }}/install.ps1 | iex

.EXAMPLE
    # Install with custom directory:
    $env:INSTALL_DIR = "C:\tools\coder"; irm {{ .Origin }}/install.ps1 | iex

.EXAMPLE
    # Dry run to see what would be installed:
    $env:DRY_RUN = "1"; irm {{ .Origin }}/install.ps1 | iex
#>

$ErrorActionPreference = "Stop"

function Write-CoderLog {
    param([string]$Message)
    Write-Host $Message
}

function Write-CoderError {
    param([string]$Message)
    Write-Host "ERROR: $Message" -ForegroundColor Red
}

function Get-CoderArch {
    $arch = [System.Runtime.InteropServices.RuntimeInformation]::OSArchitecture
    switch ($arch) {
        "X64" { return "amd64" }
        "Arm64" { return "arm64" }
        default {
            throw "Unsupported architecture: $arch"
        }
    }
}

function Get-CacheDir {
    $cacheBase = $env:LOCALAPPDATA
    if (-not $cacheBase) {
        $cacheBase = "$env:USERPROFILE\AppData\Local"
    }
    return Join-Path $cacheBase "coder\local_downloads"
}

function Invoke-Download {
    param(
        [string]$Url,
        [string]$OutFile,
        [bool]$DryRun
    )

    if ($DryRun) {
        Write-CoderLog "+ Would download: $Url"
        Write-CoderLog "  To: $OutFile"
        return
    }

    if (Test-Path $OutFile) {
        Write-CoderLog "+ Reusing $OutFile"
        return
    }

    $outDir = Split-Path $OutFile -Parent
    if (-not (Test-Path $outDir)) {
        New-Item -ItemType Directory -Path $outDir -Force | Out-Null
    }

    Write-CoderLog "+ Downloading $Url"
    $incompleteFile = "$OutFile.incomplete"

    try {
        $ProgressPreference = 'SilentlyContinue'
        Invoke-WebRequest -Uri $Url -OutFile $incompleteFile -UseBasicParsing
        Move-Item -Path $incompleteFile -Destination $OutFile -Force
    }
    catch {
        if (Test-Path $incompleteFile) {
            Remove-Item $incompleteFile -Force
        }
        throw
    }
}

function Add-ToUserPath {
    param(
        [string]$PathToAdd,
        [bool]$DryRun
    )

    $currentPath = [Environment]::GetEnvironmentVariable("PATH", "User")
    # Normalize the path to add (resolve full path and remove trailing slashes)
    $normalizedPathToAdd = [System.IO.Path]::GetFullPath($PathToAdd).TrimEnd('\', '/')
    # Normalize all existing PATH entries for comparison
    $pathEntries = $currentPath -split ";" | ForEach-Object {
        if ($_) { [System.IO.Path]::GetFullPath($_).TrimEnd('\', '/') }
    }
    if ($pathEntries -contains $normalizedPathToAdd) {
        Write-CoderLog "Directory already in PATH: $PathToAdd"
        return
    }

    if ($DryRun) {
        Write-CoderLog "+ Would add to PATH: $PathToAdd"
        return
    }

    $newPath = $currentPath + ";" + $PathToAdd
    [Environment]::SetEnvironmentVariable("PATH", $newPath, "User")
    $env:PATH = $env:PATH + ";" + $PathToAdd
    Write-CoderLog "Added to PATH: $PathToAdd"
}

function Write-PostInstall {
    param(
        [string]$InstallDir,
        [string]$BinaryName,
        [bool]$DryRun
    )

    if ($DryRun) {
        Write-Host ""
        Write-Host "Dry-run complete."
        Write-Host ""
        Write-Host "To install Coder, re-run this script without DRY_RUN set."
        return
    }

    Write-Host ""
    Write-Host "Coder {{ .Version }} installed."
    Write-Host ""
    Write-Host "The Coder binary has been placed in the following location:"
    Write-Host ""
    Write-Host "  $InstallDir\$BinaryName.exe"
    Write-Host ""

    $coderCommand = Get-Command $BinaryName -ErrorAction SilentlyContinue
    if (-not $coderCommand) {
        Write-Host "Extend your path to use Coder:"
        Write-Host ""
        Write-Host "  `$env:PATH = `"$InstallDir;`$env:PATH`""
        Write-Host ""
        Write-Host "Or restart your terminal for the PATH changes to take effect."
        Write-Host ""
        Write-Host "To run a Coder server:"
        Write-Host ""
        Write-Host "  $BinaryName server"
        Write-Host ""
        Write-Host "To connect to a Coder deployment:"
        Write-Host ""
        Write-Host "  $BinaryName login <deployment url>"
    }
    elseif ($coderCommand.Source -ne "$InstallDir\$BinaryName.exe") {
        Write-Host "Warning: There is another binary in your PATH that conflicts with the binary we've installed."
        Write-Host ""
        Write-Host "  $($coderCommand.Source)"
        Write-Host ""
        Write-Host "This is likely because of an existing installation of Coder in your PATH."
        Write-Host ""
        Write-Host "Run 'Get-Command -All $BinaryName' to view all installations."
    }
    else {
        Write-Host "To run a Coder server:"
        Write-Host ""
        Write-Host "  $BinaryName server"
        Write-Host ""
        Write-Host "To connect to a Coder deployment:"
        Write-Host ""
        Write-Host "  $BinaryName login <deployment url>"
    }
    Write-Host ""
}

function Install-CoderCLI {
    $origin = "{{ .Origin }}"
    $version = "{{ .Version }}"

    # Parse environment variables for configuration
    $dryRun = $env:DRY_RUN -eq "1"
    $installDir = $env:INSTALL_DIR
    $binaryName = $env:BINARY_NAME

    if (-not $installDir) {
        $installDir = Join-Path $env:USERPROFILE ".coder\bin"
    }
    if (-not $binaryName) {
        $binaryName = "coder"
    }

    $arch = Get-CoderArch
    $cacheDir = Get-CacheDir

    Write-CoderLog "Installing coder-windows-$arch $version from $origin."
    Write-CoderLog ""

    if ($dryRun) {
        Write-CoderLog "Running with DRY_RUN; the following are the commands that would be run if this were a real installation:"
        Write-CoderLog ""
    }

    $binaryUrl = "$origin/bin/coder-windows-$arch.exe"
    $binaryFile = Join-Path $cacheDir "coder-windows-$arch-$version.exe"

    try {
        Invoke-Download -Url $binaryUrl -OutFile $binaryFile -DryRun $dryRun
    }
    catch {
        throw "Failed to download Coder CLI: $_"
    }

    $installPath = Join-Path $installDir "$binaryName.exe"

    if ($dryRun) {
        Write-CoderLog "+ Would create directory: $installDir"
        Write-CoderLog "+ Would copy $binaryFile to $installPath"
    }
    else {
        if (-not (Test-Path $installDir)) {
            Write-CoderLog "+ Creating directory: $installDir"
            New-Item -ItemType Directory -Path $installDir -Force | Out-Null
        }

        if (Test-Path $installPath) {
            Write-CoderLog "+ Removing existing binary: $installPath"
            Remove-Item $installPath -Force
        }

        Write-CoderLog "+ Copying $binaryFile to $installPath"
        Copy-Item -Path $binaryFile -Destination $installPath -Force
    }

    Add-ToUserPath -PathToAdd $installDir -DryRun $dryRun

    Write-PostInstall -InstallDir $installDir -BinaryName $binaryName -DryRun $dryRun
}

# Run the installer
Install-CoderCLI
