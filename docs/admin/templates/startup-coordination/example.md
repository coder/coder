# Workspace Startup Coordination Examples

## Script Example

This example shows a complete, production-ready script that starts Claude Code
only after a repository has been cloned. It includes error handling, graceful
degradation, and cleanup on exit:

```bash
#!/bin/bash
set -euo pipefail

UNIT_NAME="claude-code"
DEPENDENCIES="git-clone"
REPO_DIR="/workspace/repo"

# Track if sync started successfully
SYNC_STARTED=0

# Declare dependencies
if [ -n "$DEPENDENCIES" ]; then
  if command -v coder > /dev/null 2>&1; then
    IFS=',' read -ra DEPS <<< "$DEPENDENCIES"
    for dep in "${DEPS[@]}"; do
      dep=$(echo "$dep" | xargs)
      if [ -n "$dep" ]; then
        echo "Waiting for dependency: $dep"
        coder exp sync want "$UNIT_NAME" "$dep" > /dev/null 2>&1 || \
          echo "Warning: Failed to register dependency $dep, continuing..."
      fi
    done
  else
    echo "Coder CLI not found, running without sync coordination"
  fi
fi

# Start sync and track success
if [ -n "$UNIT_NAME" ]; then
  if command -v coder > /dev/null 2>&1; then
    if coder exp sync start "$UNIT_NAME" > /dev/null 2>&1; then
      SYNC_STARTED=1
      echo "Started sync: $UNIT_NAME"
    else
      echo "Sync start failed or not available, continuing without sync..."
    fi
  fi
fi

# Ensure completion on exit (even if script fails)
cleanup_sync() {
  if [ "$SYNC_STARTED" -eq 1 ] && [ -n "$UNIT_NAME" ]; then
    echo "Completing sync: $UNIT_NAME"
    coder exp sync complete "$UNIT_NAME" > /dev/null 2>&1 || \
      echo "Warning: Sync complete failed, but continuing..."
  fi
}
trap cleanup_sync EXIT

# Now do the actual work
echo "Repository cloned, starting Claude Code"
cd "$REPO_DIR"
claude
```

This script demonstrates several [best practices](./usage.md#best-practices):

- Checking for Coder CLI availability before using sync commands
- Tracking whether `coder exp sync` started successfully
- Using `trap` to ensure completion even if the script exits early
- Graceful degradation when `coder exp sync` isn't available
- Redirecting `coder exp sync` output to reduce noise in logs


## Template Migration Example

Below is a simple example Docker template that clones [Miguel Grinberg's example Flask repo](https://github.com/miguelgrinberg/microblog/) using the [`git-clone` module](https://registry.coder.com/modules/coder/git-clone) and installs the required dependencies for the project:

- Python development headers (required for building some Python packages)
- Python dependencies from the project's `requirements.txt`

We've omitted some details (such as persistent storage) for brevity, but these are easily added.

### Before

```terraform
data "coder_provisioner" "me" {}
data "coder_workspace" "me" {}
data "coder_workspace_owner" "me" {}

resource "docker_container" "workspace" {
  count = data.coder_workspace.me.start_count
  image = "codercom/enterprise-base:ubuntu"
  name = "coder-${data.coder_workspace_owner.me.name}-${lower(data.coder_workspace.me.name)}"
  entrypoint = ["sh", "-c", coder_agent.main.init_script]
  env        = [
    "CODER_AGENT_TOKEN=${coder_agent.main.token}",
    ]
}

resource "coder_agent" "main" {
  arch           = data.coder_provisioner.me.arch
  os             = "linux"
}

module "git-clone" {
  count             = data.coder_workspace.me.start_count
  source            = "registry.coder.com/coder/git-clone/coder"
  version           = "1.2.3"
  agent_id          = coder_agent.main.id
  url               = "https://github.com/miguelgrinberg/microblog"
}

resource "coder_script" "setup" {
  count              = data.coder_workspace.me.start_count
  agent_id           = coder_agent.main.id
  display_name       = "Installing Dependencies"
  run_on_start       = true
  script             = <<EOT
    sudo apt-get update
    sudo apt-get install --yes python-dev-is-python3
    cd ${module.git-clone[count.index].repo_dir}
    python3 -m venv .venv
    source .venv/bin/activate
    pip install -r requirements.txt
  EOT
}
```

We can note the following issues in the above template:

1. There is a race between cloning the repository and the `pip install` commands, which can lead to failed workspace startups in some cases.
2. The `apt` commands can run independently of the `git clone` command, meaning that there is a potential speedup here.

Based on the above, we can improve both the startup time and reliability of the template by splitting the monolithic startup script into multiple independent scripts:

- Install `apt` dependencies
- Install `pip` dependencies (depends on the `git-clone` module and the above step)

### After

Here is the updated version of the template:

```terraform
data "coder_provisioner" "me" {}
data "coder_workspace" "me" {}
data "coder_workspace_owner" "me" {}

resource "docker_container" "workspace" {
  count = data.coder_workspace.me.start_count
  image = "codercom/enterprise-base:ubuntu"
  name = "coder-${data.coder_workspace_owner.me.name}-${lower(data.coder_workspace.me.name)}"
  entrypoint = ["sh", "-c", coder_agent.main.init_script]
  env        = [
    "CODER_AGENT_TOKEN=${coder_agent.main.token}",
    "CODER_AGENT_SOCKET_SERVER_ENABLED=true"
    ]
}

resource "coder_agent" "main" {
  arch           = data.coder_provisioner.me.arch
  os             = "linux"
}

module "git-clone" {
  count             = data.coder_workspace.me.start_count
  source            = "registry.coder.com/coder/git-clone/coder"
  version           = "1.2.3"
  agent_id          = coder_agent.main.id
  url               = "https://github.com/miguelgrinberg/microblog/"
  post_clone_script = <<-EOT
    coder exp sync start git-clone && coder exp sync complete git-clone
  EOT
}

resource "coder_script" "apt-install" {
  count              = data.coder_workspace.me.start_count
  agent_id           = coder_agent.main.id
  display_name       = "Installing APT Dependencies"
  run_on_start       = true
  script             = <<EOT
    trap 'coder exp sync complete apt-install' EXIT
    coder exp sync start apt-install

    sudo apt-get update
    sudo apt-get install --yes python-dev-is-python3
  EOT
}

resource "coder_script" "pip-install" {
  count              = data.coder_workspace.me.start_count
  agent_id           = coder_agent.main.id
  display_name       = "Installing Python Dependencies"
  run_on_start       = true
  script             = <<EOT
    trap 'coder exp sync complete pip-install' EXIT
    coder exp sync want pip-install git-clone apt-install
    coder exp sync start pip-install

    cd ${module.git-clone[count.index].repo_dir}
    python3 -m venv .venv
    source .venv/bin/activate
    pip install -r requirements.txt
  EOT
}
```

A short summary of the changes:

- We've added `CODER_AGENT_SOCKET_SERVER_ENABLED=true` to the environment variables of the Docker container in which the Coder agent runs.
- We've broken the monolithic "setup" script into two separate scripts: one for the `apt` commands, and one for the `pip` commands.
	- In each script, we've added a `coder exp sync start $SCRIPT_NAME` command to mark the startup script as started.
	- We've also added an exit trap to ensure that we mark the startup scripts as completed. Without this, the `coder exp sync wait` command would eventually time out.
- We have used the `post_clone_script` feature of the `git-clone` module to allow waiting on the Git repository clone.
- In the `pip-install` script, we have declared a dependency on both `git-clone` and `apt-install`.

With these changes, the startup time has been reduced significantly and there is no longer any possibility of a race condition.
