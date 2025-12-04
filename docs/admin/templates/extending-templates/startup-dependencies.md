# Startup Dependencies in Templates

When workspaces start, scripts often need to run in a specific order.
For example, an IDE or coding agent might need the repository cloned
before it can start. Without explicit coordination, these scripts can
race against each other, leading to startup failures and inconsistent
workspace states.

Coder's workspace startup coordination feature allows scripts to declare
dependencies on each other, ensuring they execute in the correct order. This
eliminates race conditions and makes workspace startup predictable and
reliable.

> [!NOTE]
> This feature is experimental and available under the `coder exp sync`
> command namespace.

## Benefits

- **Eliminates race conditions** during workspace startup
- **Explicit ordering** makes initialization predictable and debuggable

## What is Startup Coordination?

Startup coordination is built around the concept of **units**. A unit is a
named phase of work, typically corresponding to a script or initialization
task. Units can declare dependencies on other units, creating an explicit
ordering for workspace initialization.

**Common use case:**

- Repository must be available before starting development tools

## Requirements

To use startup dependencies in your templates:

- Coder agent with socket server enabled
- Scripts or modules that use `coder exp sync` commands.

## Enabling the Agent Socket Server

The agent socket server provides the communication layer for startup
coordination. Enable it by setting an environment variable in your
`coder_agent` resource.

To enable the agent socket server, set the appropriate environment variable within your workspace,
as shown in the partial template snippets below:

### Platform-Specific Examples

The method for passing environment variables to the agent depends on your
infrastructure platform.

<div class="tabs">

### Docker

```hcl
resource "docker_container" "workspace" {
  count = data.coder_workspace.me.start_count
  image = "codercom/enterprise-base:ubuntu"
  name  = "coder-${data.coder_workspace_owner.me.name}-${lower(data.coder_workspace.me.name)}"

  env = [
    "CODER_AGENT_SOCKET_SERVER_ENABLED=true"
  ]

  command = ["sh", "-c", coder_agent.main.init_script]
}
```

### Kubernetes

```hcl
resource "kubernetes_pod" "main" {
  count = data.coder_workspace.me.start_count

  metadata {
    name      = "coder-${data.coder_workspace_owner.me.name}-${lower(data.coder_workspace.me.name)}"
    namespace = var.workspaces_namespace
  }

  spec {
    container {
      name    = "dev"
      image   = "codercom/enterprise-base:ubuntu"
      command = ["sh", "-c", coder_agent.main.init_script]

      env {
        name  = "CODER_AGENT_SOCKET_SERVER_ENABLED"
        value = "true"
      }
    }
  }
}
```

### AWS EC2 / VMs

For virtual machines, pass the environment variable through cloud-init or your
provisioning system:

```hcl
locals {
  agent_env = {
    "CODER_AGENT_SOCKET_SERVER_ENABLED"    = "true"
  }
}

# In your cloud-init userdata template:
# %{ for key, value in local.agent_env ~}
# export ${key}="${value}"
# %{ endfor ~}
```

</div>

## Using coder_script with Dependencies

The `coder_script` resource with `run_on_start = true` is the recommended way
to implement coordinated startup. Scripts can declare dependencies and the
agent ensures proper ordering.

### Quick Start

Here's a simple example of a script that depends on another unit completing
first:

```bash
#!/bin/bash
UNIT_NAME="my-setup"

# Declare dependency on git-clone
coder exp sync want "$UNIT_NAME" "git-clone"

# Wait for dependencies and mark as started
coder exp sync start "$UNIT_NAME"

# Do your work here
echo "Running after git-clone completes"

# Signal completion
coder exp sync complete "$UNIT_NAME"
```

This script will wait until the `git-clone` unit completes before starting its
own work.

### Handling Multiple Dependencies

If your unit depends on multiple other units, you can declare all dependencies
before starting:

```bash
#!/bin/bash
UNIT_NAME="my-app"
DEPENDENCIES="git-clone,env-setup,database-migration"

# Declare all dependencies
if [ -n "$DEPENDENCIES" ]; then
  IFS=',' read -ra DEPS <<< "$DEPENDENCIES"
  for dep in "${DEPS[@]}"; do
    dep=$(echo "$dep" | xargs)  # Trim whitespace
    if [ -n "$dep" ]; then
      coder exp sync want "$UNIT_NAME" "$dep"
    fi
  done
fi

# Wait for all dependencies
coder exp sync start "$UNIT_NAME"

# Your work here
echo "All dependencies satisfied, starting application"

# Signal completion
coder exp sync complete "$UNIT_NAME"
```

### Complete Example: Claude Code Depending on Git Clone

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
claude code
```

This script demonstrates several best practices:

- Checking for Coder CLI availability before using sync commands
- Tracking whether sync started successfully
- Using `trap` to ensure completion even if the script exits early
- Graceful degradation when sync isn't available
- Redirecting sync output to reduce noise in logs

## Dependency Graph Design

### Best Practices

**Handle missing CLI gracefully.** Not all workspaces will have sync enabled. Check for the Coder CLI before using
sync commands:

```bash
if command -v coder > /dev/null 2>&1; then
  coder exp sync start "$UNIT_NAME"
else
  echo "Sync not available, continuing without coordination"
fi
```

**Always complete units that start successfully.** Units must call `complete` to unblock dependent units. Use `trap` to ensure
completion even if your script exits early or encounters errors:

```bash

SYNC_STARTED=0
if coder exp sync start "$UNIT_NAME"; then
  SYNC_STARTED=1
fi

cleanup_sync() {
  if [ "$SYNC_STARTED" -eq 1 ]; then
    coder exp sync complete "$UNIT_NAME"
  fi
}
trap cleanup_sync EXIT
```

**Use descriptive unit names.** Names should explain what the unit does, not
its position in a sequence:

- Good: `git-clone`, `env-setup`, `database-migration`
- Avoid: `step1`, `init`, `script-1`

**Prefix a unique name to your units.** Unit names like `git-clone` might
be common. Prefix the name of your organization or module to your units to
ensure that your unit does not conflict with others.

- Good: `<org>.git-clone`, `<module>.claude`
- Bad: `git-clone`, `claude`

**Document dependencies.** Add comments explaining why dependencies exist:

```hcl
resource "coder_script" "ide_setup" {
  # Depends on git-clone because we need .vscode/extensions.json
  # Depends on env-setup because we need $NODE_PATH configured
  script = <<-EOT
    coder exp sync want "ide-setup" "git-clone"
    coder exp sync want "ide-setup" "env-setup"
    # ...
  EOT
}
```

**Avoid circular dependencies.** The system detects and rejects cycles, but
they indicate a design problem:

```text
# This will fail
unit-a depends on unit-b
unit-b depends on unit-a
```

## Debugging Template Issues

### Testing Sync Availability

From a workspace terminal, test if sync is working:

```bash
# Test connectivity
coder exp sync ping

# List all units
coder exp sync list

# Check specific unit status
coder exp sync status git-clone
```

### Common Issues

**Socket not enabled:** Scripts fail with connection errors.

Solution: Verify `CODER_AGENT_SOCKET_SERVER_ENABLED=true` is set in agent
environment variables. Check both the `coder_agent` resource and the platform
resource (Docker container, Kubernetes pod, etc.).

**Script hangs forever:** A unit waits for dependencies that never complete.

Solution: Check workspace build logs for the dependency script. Verify it calls
`coder exp sync complete`. Look for errors that caused early exit. Ensure the
dependency uses `trap` for cleanup.

**Units never start:** Scripts don't execute or immediately fail.

Solution: Check workspace build logs for script errors. Verify the Coder CLI is
available in the workspace image. Test scripts manually in a workspace
terminal.

**Cycle detected error:** System rejects template with circular dependencies.

Solution: Review dependency declarations. Draw out the dependency graph to find
the cycle. Redesign dependencies to remove circular relationships.

### Viewing Build Logs

Check workspace build logs to see script execution and sync coordination:

```bash
# Get latest build ID
coder list --output json | \
  jq -r '.[] | select(.name=="my-workspace") | .latest_build.id'

# View build logs
coder builds logs <build-id>
```

Look for sync-related messages like "Started sync: unit-name" and "Completing
sync: unit-name".

## Migration Guide

### Step 1: Identify Scripts with Timing Dependencies

Look for these patterns in existing templates:

- `sleep` commands used to order scripts
- Scripts that fail intermittently on startup
- Comments like "must run after X" or "wait for Y"
- Race conditions between resource creation and script execution

### Step 2: Enable Socket Server

Add the environment variable to your agent:

```hcl
resource "coder_agent" "main" {
  env = {
    CODER_AGENT_SOCKET_SERVER_ENABLED = "true"
  }
}
```

Ensure it's passed to your compute resource (Docker, Kubernetes, VM).

### Step 3: Add Sync Commands to Scripts

Update scripts to use sync coordination:

**Before:**

```bash
#!/bin/bash
# Hope git clone finishes first
sleep 30
code-server --install-extension
```

**After:**

```bash
#!/bin/bash
set -euo pipefail

SYNC_STARTED=0
trap 'if [ $SYNC_STARTED -eq 1 ]; then coder exp sync complete "ide-setup"; fi' EXIT

coder exp sync want "ide-setup" "git-clone"

if coder exp sync start "ide-setup"; then
  SYNC_STARTED=1
fi

code-server --install-extension
```

### Step 4: Test with Single Workspace

Before rolling out to all users:

1. Create a test workspace from the updated template
2. Check workspace build logs for sync messages
3. Run `coder exp sync list` from the workspace terminal
4. Verify all units reach "completed" status
5. Test workspace functionality

### Step 5: Roll Out to All Workspaces

Push the new template version:

```bash
coder templates push <template-name>
```

Existing workspaces will use the new version on their next restart. Users can
manually update with:

```bash
coder update <workspace-name>
```

## Performance Considerations

**Overhead:** Sync adds minimal overhead. The default polling interval is 1
second, so waiting for dependencies adds at most a few seconds to startup.

**Parallel execution:** Units with no dependencies run immediately and in
parallel. Only units with unsatisfied dependencies wait.

**Timeout:** The default 5-minute timeout prevents indefinite hangs if
dependencies fail. Adjust timeouts for long-running operations:

```bash
coder exp sync start "long-operation" --timeout 10m
```

**No state persistence:** Sync state resets on workspace restart. This is
intentional to ensure clean initialization on every start.

## Next Steps

- [Workspace Build Timing](./troubleshooting.md) - Analyze and optimize startup
  performance
- [coder_script Documentation](https://registry.terraform.io/providers/coder/coder/latest/docs/resources/script) -
  Complete coder_script resource reference
- [Git Clone Module](https://registry.coder.com/modules/git-clone) - Registry
  module with sync support
