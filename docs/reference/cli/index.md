<!-- DO NOT EDIT | GENERATED CONTENT -->
# coder

## Usage

```console
coder [global-flags] <subcommand>
```

## Description

```console
Coder — A tool for provisioning self-hosted development environments with Terraform.
  - Start a Coder server:

     $ coder server

  - Get started by creating a template from an example:

     $ coder templates init
```

## Subcommands

| Name                                               | Purpose                                                                                               |
|----------------------------------------------------|-------------------------------------------------------------------------------------------------------|
| [<code>completion</code>](./completion.md)         | Install or update shell completion scripts for the detected or chosen shell.                          |
| [<code>dotfiles</code>](./dotfiles.md)             | Personalize your workspace by applying a canonical dotfiles repository                                |
| [<code>external-auth</code>](./external-auth.md)   | Manage external authentication                                                                        |
| [<code>login</code>](./login.md)                   | Authenticate with Coder deployment                                                                    |
| [<code>logout</code>](./logout.md)                 | Unauthenticate your local session                                                                     |
| [<code>netcheck</code>](./netcheck.md)             | Print network debug information for DERP and STUN                                                     |
| [<code>notifications</code>](./notifications.md)   | Manage Coder notifications                                                                            |
| [<code>organizations</code>](./organizations.md)   | Organization related commands                                                                         |
| [<code>port-forward</code>](./port-forward.md)     | Forward ports from a workspace to the local machine. For reverse port forwarding, use "coder ssh -R". |
| [<code>publickey</code>](./publickey.md)           | Output your Coder public key used for Git operations                                                  |
| [<code>reset-password</code>](./reset-password.md) | Directly connect to the database to reset a user's password                                           |
| [<code>state</code>](./state.md)                   | Manually manage Terraform state to fix broken workspaces                                              |
| [<code>templates</code>](./templates.md)           | Manage templates                                                                                      |
| [<code>tokens</code>](./tokens.md)                 | Manage personal access tokens                                                                         |
| [<code>users</code>](./users.md)                   | Manage users                                                                                          |
| [<code>version</code>](./version.md)               | Show coder version                                                                                    |
| [<code>autoupdate</code>](./autoupdate.md)         | Toggle auto-update policy for a workspace                                                             |
| [<code>config-ssh</code>](./config-ssh.md)         | Add an SSH Host entry for your workspaces "ssh coder.workspace"                                       |
| [<code>create</code>](./create.md)                 | Create a workspace                                                                                    |
| [<code>delete</code>](./delete.md)                 | Delete a workspace                                                                                    |
| [<code>favorite</code>](./favorite.md)             | Add a workspace to your favorites                                                                     |
| [<code>list</code>](./list.md)                     | List workspaces                                                                                       |
| [<code>open</code>](./open.md)                     | Open a workspace                                                                                      |
| [<code>ping</code>](./ping.md)                     | Ping a workspace                                                                                      |
| [<code>rename</code>](./rename.md)                 | Rename a workspace                                                                                    |
| [<code>restart</code>](./restart.md)               | Restart a workspace                                                                                   |
| [<code>schedule</code>](./schedule.md)             | Schedule automated start and stop times for workspaces                                                |
| [<code>show</code>](./show.md)                     | Display details of a workspace's resources and agents                                                 |
| [<code>speedtest</code>](./speedtest.md)           | Run upload and download tests from your machine to a workspace                                        |
| [<code>ssh</code>](./ssh.md)                       | Start a shell into a workspace                                                                        |
| [<code>start</code>](./start.md)                   | Start a workspace                                                                                     |
| [<code>stat</code>](./stat.md)                     | Show resource usage for the current workspace.                                                        |
| [<code>stop</code>](./stop.md)                     | Stop a workspace                                                                                      |
| [<code>unfavorite</code>](./unfavorite.md)         | Remove a workspace from your favorites                                                                |
| [<code>update</code>](./update.md)                 | Will update and start a given workspace if it is out of date                                          |
| [<code>whoami</code>](./whoami.md)                 | Fetch authenticated user info for Coder deployment                                                    |
| [<code>support</code>](./support.md)               | Commands for troubleshooting issues with a Coder deployment.                                          |
| [<code>server</code>](./server.md)                 | Start a Coder server                                                                                  |
| [<code>features</code>](./features.md)             | List Enterprise features                                                                              |
| [<code>licenses</code>](./licenses.md)             | Add, delete, and list licenses                                                                        |
| [<code>groups</code>](./groups.md)                 | Manage groups                                                                                         |
| [<code>provisioner</code>](./provisioner.md)       | View and manage provisioner daemons and jobs                                                          |

## Options

### --url

|             |                         |
|-------------|-------------------------|
| Type        | <code>url</code>        |
| Environment | <code>$CODER_URL</code> |

URL to a deployment.

### --debug-options

|      |                   |
|------|-------------------|
| Type | <code>bool</code> |

Print all options, how they're set, then exit.

### --token

|             |                                   |
|-------------|-----------------------------------|
| Type        | <code>string</code>               |
| Environment | <code>$CODER_SESSION_TOKEN</code> |

Specify an authentication token. For security reasons setting CODER_SESSION_TOKEN is preferred.

### --no-version-warning

|             |                                        |
|-------------|----------------------------------------|
| Type        | <code>bool</code>                      |
| Environment | <code>$CODER_NO_VERSION_WARNING</code> |

Suppress warning when client and server versions do not match.

### --no-feature-warning

|             |                                        |
|-------------|----------------------------------------|
| Type        | <code>bool</code>                      |
| Environment | <code>$CODER_NO_FEATURE_WARNING</code> |

Suppress warnings about unlicensed features.

### --header

|             |                            |
|-------------|----------------------------|
| Type        | <code>string-array</code>  |
| Environment | <code>$CODER_HEADER</code> |

Additional HTTP headers added to all requests. Provide as key=value. Can be specified multiple times.

### --header-command

|             |                                    |
|-------------|------------------------------------|
| Type        | <code>string</code>                |
| Environment | <code>$CODER_HEADER_COMMAND</code> |

An external command that outputs additional HTTP headers added to all requests. The command must output each header as `key=value` on its own line.

### -v, --verbose

|             |                             |
|-------------|-----------------------------|
| Type        | <code>bool</code>           |
| Environment | <code>$CODER_VERBOSE</code> |

Enable verbose output.

### --disable-direct-connections

|             |                                                |
|-------------|------------------------------------------------|
| Type        | <code>bool</code>                              |
| Environment | <code>$CODER_DISABLE_DIRECT_CONNECTIONS</code> |

Disable direct (P2P) connections to workspaces.

### --disable-network-telemetry

|             |                                               |
|-------------|-----------------------------------------------|
| Type        | <code>bool</code>                             |
| Environment | <code>$CODER_DISABLE_NETWORK_TELEMETRY</code> |

Disable network telemetry. Network telemetry is collected when connecting to workspaces using the CLI, and is forwarded to the server. If telemetry is also enabled on the server, it may be sent to Coder. Network telemetry is used to measure network quality and detect regressions.

### --global-config

|             |                                |
|-------------|--------------------------------|
| Type        | <code>string</code>            |
| Environment | <code>$CODER_CONFIG_DIR</code> |
| Default     | <code>~/.config/coderv2</code> |

Path to the global `coder` config directory.
