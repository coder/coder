#!/bin/bash

# Coder Auto-Deployment Script
# This script automatically updates and restarts Coder when changes are pushed to GitHub

set -euo pipefail

# Configuration
REPO_DIR="${DEPLOY_PATH:-/opt/coder}"
BACKUP_DIR="/opt/coder-backups"
LOG_FILE="/var/log/coder-deploy.log"

# Colors for output
RED='\033[0;31m'
GREEN='\033[0;32m'
YELLOW='\033[1;33m'
NC='\033[0m' # No Color

log() {
    echo -e "${GREEN}[$(date +'%Y-%m-%d %H:%M:%S')]${NC} $1" | tee -a "$LOG_FILE"
}

error() {
    echo -e "${RED}[$(date +'%Y-%m-%d %H:%M:%S')] ERROR:${NC} $1" | tee -a "$LOG_FILE"
}

warn() {
    echo -e "${YELLOW}[$(date +'%Y-%m-%d %H:%M:%S')] WARNING:${NC} $1" | tee -a "$LOG_FILE"
}

# Create backup directory if it doesn't exist
mkdir -p "$BACKUP_DIR"

log "========================================"
log "Starting Coder deployment..."
log "========================================"

# Navigate to repo directory
cd "$REPO_DIR" || { error "Failed to cd to $REPO_DIR"; exit 1; }

# Get current commit for backup naming
CURRENT_COMMIT=$(git rev-parse --short HEAD 2>/dev/null || echo "unknown")
BACKUP_NAME="coder-backup-${CURRENT_COMMIT}-$(date +%Y%m%d-%H%M%S)"

log "Current commit: $CURRENT_COMMIT"

# Create backup
log "Creating backup: $BACKUP_NAME"
mkdir -p "$BACKUP_DIR/$BACKUP_NAME"
if [ -d ".coderv2" ]; then
    cp -r ./.coderv2 "$BACKUP_DIR/$BACKUP_NAME/"
else
    warn "No .coderv2 directory to backup"
fi

# Keep only last 5 backups
log "Cleaning old backups (keeping last 5)..."
cd "$BACKUP_DIR"
ls -t | tail -n +6 | xargs -r rm -rf
cd "$REPO_DIR"

# Fetch latest changes
log "Fetching latest changes from GitHub..."
git fetch origin main || { error "Failed to fetch from origin"; exit 1; }

# Check if there are updates
BEFORE_COMMIT=$(git rev-parse HEAD)
AFTER_COMMIT=$(git rev-parse origin/main)

if [ "$BEFORE_COMMIT" = "$AFTER_COMMIT" ]; then
    log "No updates available. Already at latest commit."
    exit 0
fi

log "Updates available!"
log "  Before: $BEFORE_COMMIT"
log "  After:  $AFTER_COMMIT"

# Pull changes
log "Pulling changes..."
git reset --hard origin/main || { error "Failed to reset to origin/main"; exit 1; }

# Check if Coder is running
CODER_RUNNING=false
if systemctl is-active --quiet coder 2>/dev/null; then
    CODER_RUNNING=true
    log "Coder service is running"
elif pgrep -f "coder server" > /dev/null; then
    CODER_RUNNING=true
    log "Coder process is running (not systemd)"
else
    log "Coder is not currently running"
fi

# Stop Coder if running
if [ "$CODER_RUNNING" = true ]; then
    log "Stopping Coder..."
    if systemctl is-active --quiet coder 2>/dev/null; then
        sudo systemctl stop coder || warn "Failed to stop coder service"
    else
        pkill -f "coder server" || warn "Failed to kill coder process"
    fi
    sleep 3
fi

# Check if we need to rebuild
NEEDS_BUILD=false
if git diff --name-only "$BEFORE_COMMIT" "$AFTER_COMMIT" | grep -qE '\.(go|proto)$'; then
    NEEDS_BUILD=true
    log "Go or proto files changed - rebuild required"
fi

if git diff --name-only "$BEFORE_COMMIT" "$AFTER_COMMIT" | grep -qE '^site/'; then
    NEEDS_BUILD=true
    log "Frontend files changed - rebuild required"
fi

# Rebuild if needed
if [ "$NEEDS_BUILD" = true ]; then
    log "Building Coder..."

    # Frontend build
    if [ -d "site" ]; then
        log "Building frontend..."
        cd site
        pnpm install || { error "Frontend pnpm install failed"; exit 1; }
        pnpm build || { error "Frontend build failed"; exit 1; }
        cd ..
    fi

    # Backend build
    log "Building backend..."
    make build || { error "Backend build failed"; exit 1; }
else
    log "No rebuild necessary - only config/docs changed"
fi

# Run database migrations if needed
if git diff --name-only "$BEFORE_COMMIT" "$AFTER_COMMIT" | grep -q "coderd/database/migrations/"; then
    log "Database migrations detected - they will run on next Coder start"
    warn "Please verify migrations completed successfully in logs"
fi

# Start Coder
log "Starting Coder..."
if systemctl is-active --quiet coder 2>/dev/null || systemctl is-enabled --quiet coder 2>/dev/null; then
    sudo systemctl start coder || { error "Failed to start coder service"; exit 1; }
    sleep 5
    if systemctl is-active --quiet coder; then
        log "✅ Coder service started successfully"
    else
        error "❌ Coder service failed to start"
        exit 1
    fi
else
    warn "No systemd service found. Please start Coder manually or set up systemd service."
    warn "You can start Coder with: ./coder server"
fi

# Verify deployment
NEW_COMMIT=$(git rev-parse --short HEAD)
log "========================================"
log "✅ Deployment completed successfully!"
log "   Old commit: $CURRENT_COMMIT"
log "   New commit: $NEW_COMMIT"
log "========================================"

# Show recent commits
log "Recent changes:"
git log --oneline --no-walk HEAD | tee -a "$LOG_FILE"

exit 0
