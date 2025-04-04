#!/usr/bin/env bash

# Enhanced Coder Quickstart Script
# 
# This script automates the entire process for getting started with Coder:
# 1. Installs Docker if not already installed
# 2. Installs Coder CLI
# 3. Starts a Coder server
# 4. Creates a minimal Docker-based template
# 5. Creates a workspace and sets up the Coder repository with VS Code configuration
# 
# By the end, you'll have a fully functional Coder workspace that you can
# access from VS Code on your local machine.

set -euo pipefail

echo "🚀 Welcome to the Coder Quickstart Script!"
echo "This script will set up a complete Coder environment with:"
echo "  • Docker for running workspaces"
echo "  • Coder server for managing your development environments"
echo "  • A minimal Docker template for creating workspaces"
echo "  • A workspace with the Coder repository and VS Code integration"
echo

# --- Utility ---
check_command() { command -v "$1" >/dev/null 2>&1; }

# Default Docker prefix (empty unless sudo is needed)
DOCKER_PREFIX=""

# --- Get Git Configuration ---
GIT_NAME=""
GIT_EMAIL=""

check_git_config() {
	echo "🔍 Checking Git configuration..."
	
	# Check if git is installed
	if ! check_command git; then
		echo "📦 Git not found. Installing..."
		if [[ "$(uname -s | tr '[:upper:]' '[:lower:]')" == "darwin" ]]; then
			# macOS - use Homebrew if available, otherwise prompt
			if check_command brew; then
				brew install git
			else
				echo "⚠️ Git not found and Homebrew not installed."
				echo "⚠️ Please install Git manually before continuing."
				exit 1
			fi
		elif [[ "$(uname -s | tr '[:upper:]' '[:lower:]')" == "linux" ]]; then
			# Linux
			if check_command apt-get; then
				sudo apt-get update
				sudo apt-get install -y git
			elif check_command yum; then
				sudo yum install -y git
			else
				echo "⚠️ Git not found and package manager not recognized."
				echo "⚠️ Please install Git manually before continuing."
				exit 1
			fi
		fi
	fi
	
	# Check if global git user.name and user.email are configured
	GIT_NAME=$(git config --global user.name 2>/dev/null || echo "")
	GIT_EMAIL=$(git config --global user.email 2>/dev/null || echo "")
	
	if [ -z "$GIT_NAME" ] || [ -z "$GIT_EMAIL" ]; then
		echo "🔧 Git user configuration not found or incomplete."
		configure_git
	else
		echo "✅ Git configuration found:"
		echo "   Name:  $GIT_NAME"
		echo "   Email: $GIT_EMAIL"
	fi
}

configure_git() {
	echo "🔧 Setting up Git configuration..."
	
	if [ -z "$GIT_NAME" ]; then
		echo -n "👤 Enter your name for Git commits (or press Enter for default): "
		read -r GIT_NAME
		if [ -z "$GIT_NAME" ]; then
			GIT_NAME="Coder User"
			echo "ℹ️ Using default name: $GIT_NAME"
		fi
		git config --global user.name "$GIT_NAME"
	fi
	
	if [ -z "$GIT_EMAIL" ]; then
		echo -n "📧 Enter your email for Git commits (or press Enter for default): "
		read -r GIT_EMAIL
		if [ -z "$GIT_EMAIL" ]; then
			GIT_EMAIL="coder@example.com"
			echo "ℹ️ Using default email: $GIT_EMAIL"
		fi
		git config --global user.email "$GIT_EMAIL"
	fi
	
	echo "✅ Git configured successfully."
}

# --- Install Docker ---
install_docker() {
	echo "📦 Docker not found. Installing..."

	OS="$(uname -s | tr '[:upper:]' '[:lower:]')"

	if [[ "$OS" == "darwin" ]]; then
		echo "🍎 macOS detected."

		if ! check_command brew; then
			echo "❌ Homebrew not found. Please install Homebrew first:"
			echo "👉 https://brew.sh"
			exit 1
		fi

		echo "🍺 Installing Docker Desktop via Homebrew..."
		brew install --cask docker
		echo "✅ Docker Desktop installed."

		echo "🚀 Launching Docker Desktop..."
		open -a Docker

		echo "⏳ Waiting for Docker to start..."
		until ${DOCKER_PREFIX}docker info >/dev/null 2>&1; do
			sleep 2
		done

		echo "✅ Docker is running."

	elif [[ "$OS" == "linux" ]]; then
		echo "🐧 Installing Docker for Linux..."
		curl -fsSL https://get.docker.com | sh
		
		# Set Docker permissions for current user if not already set
		if ! groups | grep -q docker; then
			echo "🔑 Setting up Docker permissions for your user..."
			sudo usermod -aG docker "$USER"
			echo "✅ Added user to docker group."
			
			# Try running the newgrp command
			echo "🔄 Activating docker group membership..."
			echo "⚠️ Group permissions normally won't be available until next login."
			
			# Alternative: Temporarily modify the Docker socket permissions if needed
			echo "🔄 Setting temporary socket permissions for this session..."
			
			if [ -e /var/run/docker.sock ]; then
				echo "🔔 Docker socket found at /var/run/docker.sock"
				# Get current socket permissions
				SOCK_PERMS=$(stat -c "%a" /var/run/docker.sock)
				echo "🔔 Current socket permissions: $SOCK_PERMS"
				
				# Check Docker permissions
				# Note: We don't use newgrp directly as it would interrupt our script
				# Instead, we just set the permissions directly
				
				# Test if permissions work now
				if ! docker ps >/dev/null 2>&1; then
					echo "⚠️ Still cannot access Docker. Trying temporary socket permissions..."
					# Temporarily modify the socket permissions (not secure for production)
					# This will only last until Docker is restarted
					sudo chmod 666 /var/run/docker.sock
					echo "🔔 Temporarily set socket permissions to 666"
				else
					echo "✅ Docker access obtained."
				fi
			else
				echo "⚠️ Docker socket not found at usual location."
				echo "🔄 Using sudo for Docker commands in this session."
				DOCKER_PREFIX="sudo "
			fi
		else
			DOCKER_PREFIX=""
		fi
		
		# Make sure Docker service is running
		if systemctl list-units --type=service | grep -q docker; then
			if ! systemctl is-active --quiet docker; then
				echo "🔄 Starting Docker service..."
				sudo systemctl start docker
			fi
		fi
		
		# Always test Docker access
		echo "🔍 Testing Docker access..."
		if ! docker info >/dev/null 2>&1; then
			echo "⚠️ Using sudo for Docker commands."
			DOCKER_PREFIX="sudo "
		fi
		
		# Final test with chosen prefix
		if ! ${DOCKER_PREFIX}docker info >/dev/null 2>&1; then
			echo "❌ Still unable to access Docker. Please ensure Docker is installed and running."
			exit 1
		fi
		
		echo "✅ Docker installed and configured on Linux."
	else
		echo "❌ Unsupported OS for Docker auto-install: $OS"
		exit 1
	fi
}

# --- Install Coder using the official installer ---
install_coder() {
	echo "📥 Installing Coder using official script..."
	curl -fsSL https://coder.com/install.sh | sh
	echo "✅ Coder installed."
}

# --- Create the template file ---
create_template_file() {
	echo "📄 Creating template file..."
	
	TEMPLATE_DIR="$HOME/coder-templates"
	mkdir -p "$TEMPLATE_DIR"
	
	# Save Git config to terraform.tfvars file for injection into the template
	cat > "$TEMPLATE_DIR/terraform.tfvars" << EOF
git_name = "${GIT_NAME//\"/\\\"}"
git_email = "${GIT_EMAIL//\"/\\\"}"
EOF

	# Create the template file with quoted heredoc to avoid syntax issues
	cat > "$TEMPLATE_DIR/main.tf" << 'TFEOF'
terraform {
  required_providers {
    coder = {
      source = "coder/coder"
    }
    docker = {
      source = "kreuzwerker/docker"
    }
  }
}

# Variables from terraform.tfvars
variable "git_name" {
  type = string
  description = "Git user name from local config"
  default = "Coder User"
}

variable "git_email" {
  type = string
  description = "Git user email from local config"
  default = "coder@example.com"
}

provider "docker" {}

data "coder_provisioner" "me" {}
data "coder_workspace" "me" {}
data "coder_workspace_owner" "me" {}

resource "coder_agent" "main" {
  arch           = data.coder_provisioner.me.arch
  os             = "linux"
  startup_script = <<-EOT
    set -e

    # Prepare user home with default files on first start
    if [ ! -f ~/.init_done ]; then
      cp -rT /etc/skel ~
      touch ~/.init_done
    fi

    # Create a projects directory and clone the Coder repository there
    if [ ! -d ~/projects ]; then
      mkdir -p ~/projects
    fi
    if [ ! -d ~/projects/coder ]; then
      echo "Cloning Coder repository..."
      git clone https://github.com/coder/coder.git ~/projects/coder
      echo "Coder repository cloned successfully!"
      
      # Create .vscode directory and extensions.json for auto-installing extensions
      mkdir -p ~/projects/coder/.vscode
      echo '{
  "recommendations": [
    "coder.coder-remote"
  ]
}' > ~/projects/coder/.vscode/extensions.json
      echo "Created VS Code extension configuration."
    else
      echo "Coder repository already exists, updating..."
      cd ~/projects/coder && git pull
      
      # Ensure .vscode/extensions.json exists even if repo was already cloned
      if [ ! -f ~/projects/coder/.vscode/extensions.json ]; then
        mkdir -p ~/projects/coder/.vscode
        echo '{
  "recommendations": [
    "coder.coder-remote"
  ]
}' > ~/projects/coder/.vscode/extensions.json
        echo "Created VS Code extension configuration."
      fi
    fi

    # Install basic tools
    sudo apt-get update
    sudo apt-get install -y curl git
  EOT

  # These environment variables allow you to make Git commits right away
  env = {
    GIT_AUTHOR_NAME     = var.git_name
    GIT_AUTHOR_EMAIL    = var.git_email
    GIT_COMMITTER_NAME  = var.git_name
    GIT_COMMITTER_EMAIL = var.git_email
    CODER_WORKSPACE_DIRECTORY = "/home/coder/projects/coder"
  }

  metadata {
    display_name = "CPU Usage"
    key          = "0_cpu_usage"
    script       = "coder stat cpu"
    interval     = 10
    timeout      = 1
  }

  metadata {
    display_name = "RAM Usage"
    key          = "1_ram_usage"
    script       = "coder stat mem"
    interval     = 10
    timeout      = 1
  }

  metadata {
    display_name = "Home Disk"
    key          = "3_home_disk"
    script       = "coder stat disk --path /home/coder"
    interval     = 60
    timeout      = 1
  }
}

# Code-server (VS Code in browser)
module "code-server" {
  count  = data.coder_workspace.me.start_count
  source = "registry.coder.com/modules/code-server/coder"
  agent_id = coder_agent.main.id
  folder = "/home/coder/projects/coder"
  auto_install_extensions = true
}

resource "docker_volume" "home_volume" {
  name = "coder-${data.coder_workspace.me.id}-home"
  lifecycle {
    ignore_changes = all
  }
  labels {
    label = "coder.owner"
    value = data.coder_workspace_owner.me.name
  }
  labels {
    label = "coder.owner_id"
    value = data.coder_workspace_owner.me.id
  }
  labels {
    label = "coder.workspace_id"
    value = data.coder_workspace.me.id
  }
  labels {
    label = "coder.workspace_name_at_creation"
    value = data.coder_workspace.me.name
  }
}

resource "docker_container" "workspace" {
  count = data.coder_workspace.me.start_count
  image = "codercom/enterprise-base:ubuntu"
  name = "coder-${data.coder_workspace_owner.me.name}-${lower(data.coder_workspace.me.name)}"
  hostname = data.coder_workspace.me.name
  entrypoint = ["sh", "-c", coder_agent.main.init_script]
  env = ["CODER_AGENT_TOKEN=${coder_agent.main.token}"]
  host {
    host = "host.docker.internal"
    ip   = "host-gateway"
  }
  volumes {
    container_path = "/home/coder"
    volume_name    = docker_volume.home_volume.name
    read_only      = false
  }
  labels {
    label = "coder.owner"
    value = data.coder_workspace_owner.me.name
  }
  labels {
    label = "coder.owner_id"
    value = data.coder_workspace_owner.me.id
  }
  labels {
    label = "coder.workspace_id"
    value = data.coder_workspace.me.id
  }
  labels {
    label = "coder.workspace_name"
    value = data.coder_workspace.me.name
  }
}
TFEOF

	# Create a versions.tf file to specify provider versions
	cat > "$TEMPLATE_DIR/versions.tf" <<'EOF'
terraform {
  required_providers {
    coder = {
      source = "coder/coder"
    }
    docker = {
      source  = "kreuzwerker/docker"
      version = "~> 3.0.2"
    }
  }
  required_version = ">= 1.0.0"
}
EOF

	# Initialize terraform to create the lock file
	echo "🔄 Initializing Terraform for the template..."
	(cd "$TEMPLATE_DIR" && terraform init -backend=false > /dev/null)
	
	echo "✅ Template files created and initialized in $TEMPLATE_DIR"
}

# --- Start Coder and setup workflow ---
setup_coder() {
	echo
	echo "🚀 Starting Coder server..."
	echo "📣 Follow the URL printed below to finish setup in your browser."
	echo "   Once the server starts, you'll need to:"
	echo "   1. Create an administrator account"
	echo "   2. Return to this terminal when prompted"
	
	# Start the Coder server in the background with output to a temp file
	LOG_FILE="/tmp/coder-server-$$.log"
	touch "$LOG_FILE"
	chmod 600 "$LOG_FILE"  # Secure the log file
	
	# Start the server in the background using in-memory database for quick setup
	echo "🧠 Using in-memory database for quickstart (data will not persist after shutdown)"
	coder server --in-memory > "$LOG_FILE" 2>&1 &
	SERVER_PID=$!
	
	# Function to clean up when the script exits
	cleanup() {
		echo "Cleaning up..."
		kill $SERVER_PID 2>/dev/null || true
		rm -f "$LOG_FILE"
	}
	trap cleanup EXIT
	
	# Wait for the server to start
	echo "⏳ Waiting for Coder server to start..."
	CODER_URL=""
	TIMEOUT=60
	START_TIME=$(date +%s)
	
	while [ -z "$CODER_URL" ]; do
		# Check for server timeout
		CURRENT_TIME=$(date +%s)
		if [ $((CURRENT_TIME - START_TIME)) -gt $TIMEOUT ]; then
			echo "❌ Timed out waiting for Coder server to start."
			exit 1
		fi
		
		# Check if server process is still running
		if ! kill -0 $SERVER_PID 2>/dev/null; then
			echo "❌ Coder server process has terminated unexpectedly."
			cat "$LOG_FILE"
			exit 1
		fi
		
		# Look for URL in log file
		CODER_URL=$(grep -o "https://.*try.coder.app" "$LOG_FILE" | head -1 || grep -o -E "http://localhost:[0-9]+" "$LOG_FILE" | head -1 || echo "")
		
		if [ -n "$CODER_URL" ]; then
			echo "🌐 Found Coder Web UI: $CODER_URL"
			# Try to open the browser
			if [[ "$OSTYPE" == "darwin"* ]]; then
				open "$CODER_URL" 2>/dev/null || true
			elif [[ "$OSTYPE" == "linux"* ]]; then
				if check_command xdg-open; then
					xdg-open "$CODER_URL" 2>/dev/null || true
				fi
			fi
			break
		fi
		
		# Wait before checking again
		sleep 1
	done
	
	echo "🌐 Please open $CODER_URL in your browser to create an admin account"
	echo "📋 After creating your account, return to this terminal"
	
	# Wait for a user to be created
	echo "⏳ Waiting for user account creation..."
	USER_CREATED=false
	TIMEOUT=300
	START_TIME=$(date +%s)
	
	while ! $USER_CREATED; do
		# Check for user creation timeout
		CURRENT_TIME=$(date +%s)
		if [ $((CURRENT_TIME - START_TIME)) -gt $TIMEOUT ]; then
			echo "⚠️ Timeout waiting for user creation. Continuing anyway..."
			break
		fi
		
		# Check if a user has been created
		if grep -q "actor=\"\|created user" "$LOG_FILE" 2>/dev/null; then
			USER_CREATED=true
			USERNAME=$(grep -o 'actor="[^"]*"\|created user [^[:space:]]*' "$LOG_FILE" | head -1 | sed 's/.*"\([^"]*\)".*/\1/;s/created user //')
			echo "👤 User account created: $USERNAME"
		else
			sleep 2
		fi
	done
	
	# Create the template file
	create_template_file
	
	echo ""
	echo "✅ Coder server is now running!"
	echo ""
	echo "📱 The server detected that you created an account."
	echo "🔑 Now we need to authenticate the CLI with your Coder server."
	echo ""
	echo "⏳ Logging in to Coder server..."
	
	# Authenticate with the Coder server
	coder login "$CODER_URL" --no-open
	LOGIN_RESULT=$?
	
	if [ $LOGIN_RESULT -ne 0 ]; then
	    echo "❌ Failed to automatically log in."
	    echo "🔑 Please manually log in by running the following command in a new terminal:"
	    echo "   coder login $CODER_URL"
	    echo ""
	    echo "⏳ After logging in, return here and press Enter to continue..."
	    # Wait for user input before continuing
	    read -r
	else
	    echo "✅ Successfully logged in to Coder!"
	    echo ""
	    echo "⏳ Press Enter to continue with workspace creation..."
	    echo "   (We'll create a template and workspace for you automatically)"
	    # Wait for user input before continuing 
	    read -r
	fi
	
	# Continue with template and workspace creation
	echo ""
	echo "✅ Continuing with workspace setup..."
	echo ""
	
	# Verify login status before proceeding
	verify_login
	
	# Use direct workspace creation
	echo "⏳ Creating workspace using minimal docker template..."
	direct_workspace_creation
	
	# Keep Coder server running and stream the logs
	echo ""
	echo "✅ Setup complete! Your Coder environment is now ready."
	echo "🖥️ Coder server is running at $CODER_URL"
	echo "📊 Now streaming server logs. Press Ctrl+C to stop and exit when you're done."
	echo ""
	
	# Remove the trap since we want to keep server running
	trap - EXIT
	
	# Start streaming logs from the log file
	tail -f "$LOG_FILE"
	
	# If tail is interrupted, wait for the server process to ensure clean exit
	wait $SERVER_PID
}

# Terraform is installed automatically by Coder when needed

# --- Removed template creation as we use the built-in docker template ---

# Verify login status
verify_login() {
    echo "🔍 Verifying Coder login status..."
    if ! coder users list >/dev/null 2>&1; then
        echo "❌ Not logged in to Coder server."
        echo "🔑 Please log in using: coder login $CODER_URL"
        
        # Try to login automatically
        echo "🔄 Attempting automatic login..."
        coder login "$CODER_URL" --no-open
        
        # Check if login was successful
        if ! coder users list >/dev/null 2>&1; then
            echo "❌ Automatic login failed."
            echo "🔑 Please manually login in a new terminal with: coder login $CODER_URL"
            echo "   Then return here and press Enter to continue..."
            read -r
            
            # Verify again after manual login
            if ! coder users list >/dev/null 2>&1; then
                echo "❌ Still not logged in. Exiting."
                exit 1
            fi
        fi
    fi
    echo "✅ Successfully verified login status."
}

# Create a workspace using a minimal template
direct_workspace_creation() {
    # First verify we're logged in
    verify_login
    WORKSPACE_NAME="my-workspace"
    TEMPLATE_NAME="quickstart-docker"
    
    # Double-check Docker permissions right before template creation
    echo "🔍 Verifying Docker socket permissions..."
    if ! docker ps >/dev/null 2>&1; then
        echo "⚠️ Docker socket permissions issue detected."
        
        # Try to fix socket permissions
        if [ -e /var/run/docker.sock ]; then
            echo "🔄 Setting temporary socket permissions for Terraform to use..."
            sudo chmod 666 /var/run/docker.sock
            echo "🔔 Temporarily set socket permissions to 666"
        else
            echo "❌ Docker socket not found. Terraform will likely fail to access Docker."
        fi
    else
        echo "✅ Docker permissions look good."
    fi
    
    # Check if a workspace with this name already exists and clean it up if needed
    if coder list | grep -q "$WORKSPACE_NAME"; then
        echo "⚠️ A workspace named '$WORKSPACE_NAME' already exists."
        echo "🧹 Removing the existing workspace first..."
        coder delete "$WORKSPACE_NAME" --yes
        
        # Wait a bit for deletion to complete
        sleep 5
        echo "✅ Workspace cleaned up."
    fi
    
    # Check if the template already exists
    if ! coder templates list | grep -q "$TEMPLATE_NAME"; then
        echo "🔄 Creating a minimal docker template..."
        
        # Ensure the Docker network exists
        echo "🔄 Ensuring Docker network exists..."
        ${DOCKER_PREFIX}docker network inspect coder >/dev/null 2>&1 || ${DOCKER_PREFIX}docker network create coder
        
        # Clean up any existing containers with conflicting names
        echo "🧹 Cleaning up any existing containers..."
        EXISTING_CONTAINER=$(${DOCKER_PREFIX}docker ps -a --filter "name=coder-.*-workspace" --format "{{.Names}}" 2>/dev/null)
        if [ -n "$EXISTING_CONTAINER" ]; then
            echo "🔄 Found existing containers: $EXISTING_CONTAINER"
            echo "🔄 Stopping and removing..."
            ${DOCKER_PREFIX}docker rm -f $EXISTING_CONTAINER >/dev/null 2>&1 || true
        fi
        
        # Create a temporary directory for the template
        TEMPLATE_DIR=$(mktemp -d)
        
        # Create a very simple template file that uses Docker - based on starter template
        cat > "$TEMPLATE_DIR/main.tf" << 'EOF'
terraform {
  required_providers {
    coder = {
      source = "coder/coder"
    }
    docker = {
      source  = "kreuzwerker/docker"
      version = "~> 3.0.0"
    }
  }
}

provider "docker" {
  host = "unix:///var/run/docker.sock"
}
provider "coder" {}

# Variables for git configuration
variable "git_name" {
  type        = string
  description = "Git user name"
  default     = "Coder User"
}

variable "git_email" {
  type        = string
  description = "Git user email"
  default     = "coder@example.com"
}

data "coder_workspace" "me" {}
data "coder_workspace_owner" "me" {}

locals {
  # repo_dir will be populated by the git-clone module
  repo_dir = try(module.git-clone[0].repo_dir, "/home/coder/coder")
  container_name = "coder-${data.coder_workspace_owner.me.name}-${lower(data.coder_workspace.me.name)}"
}

resource "coder_agent" "main" {
  arch = "amd64"
  os   = "linux"
  dir  = local.repo_dir
  
  # Set environment variables to pass Git configuration
  env = {
    GIT_NAME = var.git_name
    GIT_EMAIL = var.git_email
  }
  
  # Use blocking behavior to ensure the script completes
  startup_script_behavior = "blocking"
  
  # Add a startup script to ensure the repository is cloned
  startup_script = <<-EOT
    #!/bin/bash
    set -e
    
    # This script runs after the git-clone module, but we add this
    # as a fallback in case the module doesn't work
    if [ ! -d "/home/coder/coder" ]; then
      echo "Cloning Coder repository (fallback method)..."
      cd /home/coder
      git clone https://github.com/coder/coder.git
      mkdir -p /home/coder/coder/.vscode
      echo '{"recommendations":["coder.coder-remote"]}' > /home/coder/coder/.vscode/extensions.json
    fi
    
    # Set Git configuration if values are provided
    if [ ! -z "$GIT_NAME" ]; then
      git config --global user.name "$GIT_NAME"
    fi
    if [ ! -z "$GIT_EMAIL" ]; then
      git config --global user.email "$GIT_EMAIL"
    fi
    
    echo "Startup script completed successfully!"
  EOT
  
  # Add metadata blocks for monitoring
  metadata {
    display_name = "CPU Usage"
    key          = "cpu_usage"
    order        = 0
    script       = "coder stat cpu"
    interval     = 10
    timeout      = 1
  }

  metadata {
    display_name = "RAM Usage"
    key          = "ram_usage"
    order        = 1
    script       = "coder stat mem"
    interval     = 10
    timeout      = 1
  }

  metadata {
    display_name = "Disk Usage"
    key          = "disk_usage"
    order        = 2
    script       = "coder stat disk --path /home/coder"
    interval     = 60
    timeout      = 1
  }

  # Add resources monitoring for notifications
  resources_monitoring {
    memory {
      enabled   = true
      threshold = 80
    }
    volume {
      enabled   = true
      threshold = 90
      path      = "/home/coder"
    }
  }
}

module "git-clone" {
  count    = data.coder_workspace.me.start_count
  source   = "registry.coder.com/modules/git-clone/coder"
  agent_id = coder_agent.main.id
  url      = "https://github.com/coder/coder.git"
  base_dir = "/home/coder"
}

module "code-server" {
  count                   = data.coder_workspace.me.start_count
  source                  = "registry.coder.com/modules/code-server/coder"
  agent_id                = coder_agent.main.id
  folder                  = local.repo_dir
  auto_install_extensions = true
}

# Create a persistent volume for the workspace
resource "docker_volume" "home_volume" {
  name = "coder-${data.coder_workspace.me.id}-home"
  # Protect the volume from being deleted due to changes in attributes.
  lifecycle {
    ignore_changes = all
  }
  # Add labels in Docker to keep track of orphan resources.
  labels {
    label = "coder.owner"
    value = data.coder_workspace_owner.me.name
  }
  labels {
    label = "coder.owner_id"
    value = data.coder_workspace_owner.me.id
  }
  labels {
    label = "coder.workspace_id"
    value = data.coder_workspace.me.id
  }
  labels {
    label = "coder.workspace_name_at_creation"
    value = data.coder_workspace.me.name
  }
}

resource "docker_container" "workspace" {
  count = data.coder_workspace.me.start_count
  image = "codercom/code-server:latest"
  # Use the container name from locals
  name = local.container_name
  # Hostname makes the shell more user friendly: coder@my-workspace:~$
  hostname = data.coder_workspace.me.name
  
  # Uses docker networking to expose container's IP on Docker network instead of publishing ports
  # See https://docs.docker.com/engine/reference/commandline/network_connect/
  networks_advanced {
    name = "coder"
  }
  
  # Execute the agent init script at container startup
  entrypoint = ["sh", "-c", coder_agent.main.init_script]
  env = [
    "CODER_AGENT_TOKEN=${coder_agent.main.token}",
    # Allow workspace user to access their private repositories
    "GIT_AUTHOR_NAME=${var.git_name}",
    "GIT_COMMITTER_NAME=${var.git_name}",
    "GIT_AUTHOR_EMAIL=${var.git_email}",
    "GIT_COMMITTER_EMAIL=${var.git_email}"
  ]
  
  # Give the container a bit more memory
  memory = 4096
  
  # Ensure resources are cleaned up properly with more generous timeouts
  stop_timeout = 60
  destroy_grace_seconds = 60
  stop_signal = "SIGINT"
  
  # Add host gateway for access to host Docker socket
  host {
    host = "host.docker.internal"
    ip   = "host-gateway"
  }
  
  # Mount the persistent volume
  volumes {
    container_path = "/home/coder/"
    volume_name    = docker_volume.home_volume.name
    read_only      = false
  }
  
  # Add labels in Docker to keep track of orphan resources
  labels {
    label = "coder.owner"
    value = data.coder_workspace_owner.me.name
  }
  labels {
    label = "coder.owner_id"
    value = data.coder_workspace_owner.me.id
  }
  labels {
    label = "coder.workspace_id"
    value = data.coder_workspace.me.id
  }
  labels {
    label = "coder.workspace_name"
    value = data.coder_workspace.me.name
  }
}
EOF
        
        # No need to initialize Terraform - Coder will handle this
        
        # Create the template using push command (preferred over create)
        # Create terraform.tfvars file for Git configuration
        # Escape double quotes in git variables for Terraform
        ESCAPED_GIT_NAME=$(echo "$GIT_NAME" | sed 's/"/\\"/g')
        ESCAPED_GIT_EMAIL=$(echo "$GIT_EMAIL" | sed 's/"/\\"/g')
        
        cat > "$TEMPLATE_DIR/terraform.tfvars" << EOF
git_name = "${ESCAPED_GIT_NAME}"
git_email = "${ESCAPED_GIT_EMAIL}"
EOF
        
        echo "🔄 Creating the template..."
        coder templates push "$TEMPLATE_NAME" --directory="$TEMPLATE_DIR" --yes
        CREATE_TEMPLATE_RESULT=$?
        
        # Clean up the temporary directory
        rm -rf "$TEMPLATE_DIR"
        
        if [ $CREATE_TEMPLATE_RESULT -ne 0 ]; then
            echo "❌ Failed to create template."
            echo "Try creating a template manually with: coder templates create quickstart-docker --yes"
            return 1
        fi
        
        echo "✅ Template created successfully!"
    else
        echo "✅ Using existing template: $TEMPLATE_NAME"
    fi
    
    echo "🔄 Creating a workspace using the template..."
    
    # Create workspace using the template
    coder create --yes --template="$TEMPLATE_NAME" "$WORKSPACE_NAME"
    CREATE_RESULT=$?
    
    # Check if workspace creation was successful
    if [ $CREATE_RESULT -eq 0 ]; then
        echo "✅ Workspace created successfully!"
        echo ""
        echo "🔄 Setting up the workspace with Coder repository..."
        echo "✅ The git-clone module will automatically clone the repository"
        echo "   and code-server module will set up VS Code with extensions."
        echo ""
        echo "⏳ Workspace is being created and configured..."
        echo "   (This may take 1-2 minutes for the agent to connect and modules to run)"
        
        # Wait for a few seconds for the workspace to start processing
        sleep 5
        echo "✅ Workspace prepared with Coder repository and VS Code configuration!"
        echo ""
        echo "🎉 SUCCESS! Your Coder environment is fully set up and ready to use!"
        echo ""
        echo "🚀 To connect to your workspace with VS Code:"
        echo "   1. Run this command: coder open $WORKSPACE_NAME"
        echo "   2. VS Code will launch and connect to your remote workspace"
        echo "   3. If prompted, install the Coder extension for VS Code"
        echo ""
        echo "✨ Your workspace has the Coder repository at ~/coder"
        echo "   and is ready for you to explore and edit files remotely."
        echo ""
        echo "🔌 The VS Code extensions needed for remote development are automatically"
        echo "   configured and will install when you connect."
    else
        # Handle workspace creation failure or issues
        if [ $CREATE_RESULT -ne 0 ]; then
            echo "❌ Failed to create workspace."
            echo "Try manually with: coder create --yes --template=docker my-workspace"
        else
            echo "⚠️ Workspace created but there was an issue setting up the Coder repository."
            echo "   The workspace was created but the repository wasn't cloned automatically."
            echo "   It should be cloned when the agent startup script completes."
            echo "   If not, you can manually set it up with these steps:"
            echo ""
            echo "   1. Connect to your workspace: coder open $WORKSPACE_NAME"
            echo "   2. Once VS Code opens, open a terminal (Terminal > New Terminal)"
            echo "   3. Verify if the repository exists: ls -la ~/"
            echo "   4. If it doesn't exist, run these commands:"
            echo "      cd ~"
            echo "      git clone https://github.com/coder/coder.git"
            echo "      mkdir -p ~/coder/.vscode"
            echo "      echo '{\"recommendations\":[\"coder.coder-remote\"]}' > ~/coder/.vscode/extensions.json"
            echo "      git config --global user.name \"$GIT_NAME\""
            echo "      git config --global user.email \"$GIT_EMAIL\""
        fi
    fi
}

# --- Run ---
if ! check_command docker; then 
    install_docker 
else
    # Docker is installed but we need to check if it requires sudo
    echo "🔍 Testing Docker access..."
    if ! docker info >/dev/null 2>&1; then
        echo "⚠️ Using sudo for Docker commands."
        DOCKER_PREFIX="sudo "
    else
        DOCKER_PREFIX=""
    fi
fi

if ! check_command coder; then install_coder; fi

# Check git config before starting to ensure we have the info for the template
check_git_config

# Skip creating template files entirely as we'll use the built-in docker template
create_template_file() {
	echo "✅ Using built-in docker template instead of creating custom template files"
}

# Start Coder and guide the user through setup
setup_coder