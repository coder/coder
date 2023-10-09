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

| Name                                                   | Purpose                                                                                               |
| ------------------------------------------------------ | ----------------------------------------------------------------------------------------------------- |
| [<code>config-ssh</code>](./cli/config-ssh.md)         | Add an SSH Host entry for your workspaces "ssh coder.workspace"                                       |
| [<code>create</code>](./cli/create.md)                 | Create a workspace                                                                                    |
| [<code>delete</code>](./cli/delete.md)                 | Delete a workspace                                                                                    |
| [<code>dotfiles</code>](./cli/dotfiles.md)             | Personalize your workspace by applying a canonical dotfiles repository                                |
| [<code>external-auth</code>](./cli/external-auth.md)   | Manage external authentication                                                                        |
| [<code>features</code>](./cli/features.md)             | List Enterprise features                                                                              |
| [<code>groups</code>](./cli/groups.md)                 | Manage groups                                                                                         |
| [<code>licenses</code>](./cli/licenses.md)             | Add, delete, and list licenses                                                                        |
| [<code>list</code>](./cli/list.md)                     | List workspaces                                                                                       |
| [<code>login</code>](./cli/login.md)                   | Authenticate with Coder deployment                                                                    |
| [<code>logout</code>](./cli/logout.md)                 | Unauthenticate your local session                                                                     |
| [<code>netcheck</code>](./cli/netcheck.md)             | Print network debug information for DERP and STUN                                                     |
| [<code>ping</code>](./cli/ping.md)                     | Ping a workspace                                                                                      |
| [<code>port-forward</code>](./cli/port-forward.md)     | Forward ports from a workspace to the local machine. For reverse port forwarding, use "coder ssh -R". |
| [<code>provisionerd</code>](./cli/provisionerd.md)     | Manage provisioner daemons                                                                            |
| [<code>publickey</code>](./cli/publickey.md)           | Output your Coder public key used for Git operations                                                  |
| [<code>rename</code>](./cli/rename.md)                 | Rename a workspace                                                                                    |
| [<code>reset-password</code>](./cli/reset-password.md) | Directly connect to the database to reset a user's password                                           |
| [<code>restart</code>](./cli/restart.md)               | Restart a workspace                                                                                   |
| [<code>schedule</code>](./cli/schedule.md)             | Schedule automated start and stop times for workspaces                                                |
| [<code>server</code>](./cli/server.md)                 | Start a Coder server                                                                                  |
| [<code>show</code>](./cli/show.md)                     | Display details of a workspace's resources and agents                                                 |
| [<code>speedtest</code>](./cli/speedtest.md)           | Run upload and download tests from your machine to a workspace                                        |
| [<code>ssh</code>](./cli/ssh.md)                       | Start a shell into a workspace                                                                        |
| [<code>start</code>](./cli/start.md)                   | Start a workspace                                                                                     |
| [<code>stat</code>](./cli/stat.md)                     | Show resource usage for the current workspace.                                                        |
| [<code>state</code>](./cli/state.md)                   | Manually manage Terraform state to fix broken workspaces                                              |
| [<code>stop</code>](./cli/stop.md)                     | Stop a workspace                                                                                      |
| [<code>templates</code>](./cli/templates.md)           | Manage templates                                                                                      |
| [<code>tokens</code>](./cli/tokens.md)                 | Manage personal access tokens                                                                         |
| [<code>update</code>](./cli/update.md)                 | Will update and start a given workspace if it is out of date                                          |
| [<code>users</code>](./cli/users.md)                   | Manage users                                                                                          |
| [<code>version</code>](./cli/version.md)               | Show coder version                                                                                    |

## Options

### --debug-options

|      |                   |
| ---- | ----------------- |
| Type | <code>bool</code> |

Print all options, how they're set, then exit.

### --disable-direct-connections

|             |                                                |
| ----------- | ---------------------------------------------- |
| Type        | <code>bool</code>                              |
| Environment | <code>$CODER_DISABLE_DIRECT_CONNECTIONS</code> |

Disable direct (P2P) connections to workspaces.

### --global-config

|             |                                |
| ----------- | ------------------------------ |
| Type        | <code>string</code>            |
| Environment | <code>$CODER_CONFIG_DIR</code> |
| Default     | <code>~/.config/coderv2</code> |

Path to the global `coder` config directory.

### --header

|             |                            |
| ----------- | -------------------------- |
| Type        | <code>string-array</code>  |
| Environment | <code>$CODER_HEADER</code> |

Additional HTTP headers added to all requests. Provide as key=value. Can be
specified multiple times.

### --header-command

|             |                                    |
| ----------- | ---------------------------------- |
| Type        | <code>string</code>                |
| Environment | <code>$CODER_HEADER_COMMAND</code> |

An external command that outputs additional HTTP headers added to all requests.
The command must output each header as `key=value` on its own line.

### --no-feature-warning

|             |                                        |
| ----------- | -------------------------------------- |
| Type        | <code>bool</code>                      |
| Environment | <code>$CODER_NO_FEATURE_WARNING</code> |

Suppress warnings about unlicensed features.

### --no-version-warning

|             |                                        |
| ----------- | -------------------------------------- |
| Type        | <code>bool</code>                      |
| Environment | <code>$CODER_NO_VERSION_WARNING</code> |

Suppress warning when client and server versions do not match.

### --token

|             |                                   |
| ----------- | --------------------------------- |
| Type        | <code>string</code>               |
| Environment | <code>$CODER_SESSION_TOKEN</code> |

Specify an authentication token. For security reasons setting
CODER_SESSION_TOKEN is preferred.

### --url

|             |                         |
| ----------- | ----------------------- |
| Type        | <code>url</code>        |
| Environment | <code>$CODER_URL</code> |

URL to a deployment.

### -v, --verbose

|             |                             |
| ----------- | --------------------------- |
| Type        | <code>bool</code>           |
| Environment | <code>$CODER_VERBOSE</code> |

Enable verbose output.
