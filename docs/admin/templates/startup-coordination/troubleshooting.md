# Workspace Startup Coordination Troubleshooting

> [!NOTE]
> This feature is experimental and may change without notice in future releases.

## Test Sync Availability

From a workspace terminal, test if sync is working using `coder exp sync ping`:

```sh
coder exp sync ping
```

* If sync is working, expect the output to be `Success`.
* Otherwise, you will see an error message similar to the below:

```sh
error: connect to agent socket: connect to socket: dial unix /tmp/coder-agent.sock: connect: permission denied
```

## Check Unit Status

You can check the status of a specific unit using `coder exp sync status`:

```sh
coder exp sync status git-clone
```

If the unit exists, you will see output similar to the below:

```sh
# coder exp sync status git-clone
Unit: git-clone
Status: completed
Ready: true
```

If the unit is not known to the agent, you will see output similar to the below:

```sh
# coder exp sync status doesnotexist
Unit: doesnotexist
Status: not registered
Ready: true

Dependencies:
No dependencies found
```

## List All Units

If you are unsure which units are registered, or want a quick overview of every unit's state, use `coder exp sync list`:

```sh
coder exp sync list
```

This displays all registered units, their statuses, and whether they are ready to start:

```sh
UNIT           STATUS     READY
git-clone      completed  true
env-setup      started    true
ide-configure  pending    false
```

You can also get JSON output for scripting:

```sh
coder exp sync list --output json
```

## Common Issues

### Workspace startup script hangs

If the workspace startup scripts appear to 'hang', one or more of your startup scripts may be waiting for a dependency that never completes.

* Inside the workspace, review `/tmp/coder-script-*.log` for more details on your script's execution.
    > **Tip:** add `set -x` to the top of your script to enable debug mode and update/restart the workspace.
* Review your template and verify that `coder exp sync complete <unit>` is called after the script completes e.g. with an exit trap.
* List all units to identify which ones are blocked: `coder exp sync list`.
* View the unit status using `coder exp sync status <unit>`.

### Workspace startup scripts fail

If the workspace startup scripts fail:

* Review `/tmp/coder-script-*.log` inside the workspace for script errors.
* Verify the Coder CLI is available in `$PATH` inside the workspace:

    ```sh
    command -v coder
    ```

### Cycle detected

If you see an error similar to the below in your startup script logs, you have defined a cyclic dependency:

```sh
error: declare dependency failed: cannot add dependency: adding edge for unit "bar": failed to add dependency
adding edge (bar -> foo): cycle detected
```

To fix this, review your dependency declarations and redesign them to remove the cycle. It may help to draw out the dependency graph to find
the cycle.
