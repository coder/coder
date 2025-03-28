# Coder Quickstart Script for Windows
# Installs Docker Desktop and Coder via winget
# Launches `coder server` and lets the user follow the printed URL

Write-Host "`n🚀 Starting Coder Quickstart for Windows`n"

function Test-Command($cmd) {
    $null -ne (Get-Command $cmd -ErrorAction SilentlyContinue)
}

# --- Install Docker Desktop ---
if (-not (Test-Command "docker")) {
    Write-Host "📦 Docker not found. Installing via winget..."
    winget install --id Docker.DockerDesktop -e --source winget
    Write-Host "✅ Docker Desktop install initiated."
    Write-Host "📣 Please start Docker Desktop manually if it doesn't launch automatically."

    # Wait until Docker is running
    Write-Host "⏳ Waiting for Docker to become available..."
    while (-not (docker info 2>$null)) {
        Start-Sleep -Seconds 2
    }
    Write-Host "✅ Docker is running."
} else {
    Write-Host "✅ Docker is already installed."
}

# --- Install Coder ---
if (-not (Test-Command "coder")) {
    Write-Host "`n📥 Installing Coder via winget..."
    winget install --id Coder.Coder -e --source winget
    Write-Host "✅ Coder installed."
} else {
    Write-Host "✅ Coder is already installed."
}

# --- Start Coder ---
Write-Host "`n🚀 Starting Coder server..."
Write-Host "📣 Follow the URL printed below to finish setup in your browser.`n"
coder server
