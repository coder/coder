# Workspace Startup Coordination Usage

> [!NOTE]
> This feature is experimental and may change without notice in future releases.

Startup coordination is built around the concept of **units**. You declare units in your Coder workspace template using the `coder exp sync` command in `coder_script` resources. When the Coder agent starts, it keeps an in-memory directed acyclic graph (DAG) of all units of which it is aware. When you need to synchronize with another unit, you can use `coder exp sync start $UNIT_NAME` to block until all dependencies of that unit have been marked complete.

## What is a unit?

A **unit** is a named phase of work, typically corresponding to a script or initialization
task.

- Units **may** declare dependencies on other units, creating an explicit ordering for workspace initialization.
- Units **must** be registered before they can be marked as complete.
- Units **may** be marked as dependencies before they are registered.
- Units **must not** declare cyclic dependencies. Attempting to create a cyclic dependency will result in an error.

## Requirements

> [!IMPORTANT]
> The `coder exp sync` command is only available from Coder version >=v2.30 onwards.

To use startup dependencies in your templates, you must:

- Enable the Coder Agent Socket Server.
- Modify your workspace startup scripts to run in parallel and declare dependencies as required using `coder exp sync`.

### Enable the Coder Agent Socket Server

The agent socket server provides the communication layer for startup
coordination. To enable it, set `CODER_AGENT_SOCKET_SERVER_ENABLED=true` in the environment in which the agent is running.
The exact method for doing this depends on your infrastructure platform:

<div class="tabs">

#### Docker / Podman

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

#### Kubernetes

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

#### AWS EC2 / VMs

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

### Declare Dependencies in your Workspace Startup Scripts

<div class="tabs">

#### Single Dependency

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

#### Multiple Dependencies

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

</div>

## Best Practices

### Test your changes before rolling out to all users

Before rolling out to all users:

1. Create a test workspace from the updated template
2. Check workspace build logs for sync messages
3. Verify all units reach "completed" status
4. Test workspace functionality

Once you're satisfied, [promote the new template version](../../../reference/cli/templates_versions_promote.md).

### Handle missing CLI gracefully

Not all workspaces will have the Coder CLI available in `$PATH`. Check for availability of the Coder CLI before using
sync commands:

```bash
if command -v coder > /dev/null 2>&1; then
  coder exp sync start "$UNIT_NAME"
else
  echo "Coder CLI not available, continuing without coordination"
fi
```

### Complete units that start successfully

Units **must** call `coder exp sync complete` to unblock dependent units. Use `trap` to ensure
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

### Use descriptive unit names

Names should explain what the unit does, not its position in a sequence:

- Good: `git-clone`, `env-setup`, `database-migration`
- Avoid: `step1`, `init`, `script-1`

### Prefix a unique name to your units

When using `coder exp sync` in modules, note that unit names like `git-clone` might be common. Prefix the name of your module to your units to
ensure that your unit does not conflict with others.

- Good: `<module>.git-clone`, `<module>.claude`
- Bad: `git-clone`, `claude`

### Document dependencies

Add comments explaining why dependencies exist:

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

### Avoid circular dependencies

The Coder Agent detects and rejects circular dependencies, but they indicate a design problem:

```bash
# This will fail
coder exp sync want "unit-a" "unit-b"
coder exp sync want "unit-b" "unit-a"
```

## Frequently Asked Questions

### How do I identify scripts that can benefit from startup coordination?

Look for these patterns in existing templates:

- `sleep` commands used to order scripts
- Using files to coordinate startup between scripts (e.g. `touch /tmp/startup-complete`)
- Scripts that fail intermittently on startup
- Comments like "must run after X" or "wait for Y"

### Will this slow down my workspace?

No. The socket server adds minimal overhead, and the default polling interval is 1
second, so waiting for dependencies adds at most a few seconds to startup.
You are more likely to notice an improvement in startup times as it becomes easier to manage complex dependencies in parallel.

### How do units interact with each other?

Units with no dependencies run immediately and in parallel.
Only units with unsatisfied dependencies wait for their dependencies.

### How long can a dependency take to complete?

By default, `coder exp sync start` has a 5-minute timeout to prevent indefinite hangs.
Upon timeout, the command will exit with an error code and print `timeout waiting for dependencies of unit <unit_name>` to stderr.

You can adjust this timeout as necessary for long-running operations:

```bash
coder exp sync start "long-operation" --timeout 10m
```

### Is state stored between restarts?

No. Sync state is kept in-memory only and resets on workspace restart.
This is intentional to ensure clean initialization on every start.
