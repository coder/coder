<!-- DO NOT EDIT | GENERATED CONTENT -->

# scaletest

Run a scale test against the Coder API

## Usage

```console
coder scaletest
```

## Subcommands

| Name                                                               | Purpose                                                                                                                                                                                                             |
| ------------------------------------------------------------------ | ------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------- |
| [<code>cleanup</code>](./scaletest_cleanup.md)                     | Cleanup scaletest workspaces, then cleanup scaletest users.                                                                                                                                                         |
| [<code>create-workspaces</code>](./scaletest_create-workspaces.md) | Creates many users, then creates a workspace for each user and waits for them finish building and fully come online. Optionally runs a command inside each workspace, and connects to the workspace over WireGuard. |
| [<code>workspace-traffic</code>](./scaletest_workspace-traffic.md) | Generate traffic to scaletest workspaces through coderd                                                                                                                                                             |
