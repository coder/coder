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

| Name                                                | Purpose                                                                |
| --------------------------------------------------- | ---------------------------------------------------------------------- |
| [<code>config-ssh</code>](./cli/config-ssh)         | Add an SSH Host entry for your workspaces "ssh coder.workspace"        |
| [<code>create</code>](./cli/create)                 | Create a workspace                                                     |
| [<code>delete</code>](./cli/delete)                 | Delete a workspace                                                     |
| [<code>dotfiles</code>](./cli/dotfiles)             | Personalize your workspace by applying a canonical dotfiles repository |
| [<code>features</code>](./cli/features)             | List Enterprise features                                               |
| [<code>groups</code>](./cli/groups)                 | Manage groups                                                          |
| [<code>licenses</code>](./cli/licenses)             | Add, delete, and list licenses                                         |
| [<code>list</code>](./cli/list)                     | List workspaces                                                        |
| [<code>login</code>](./cli/login)                   | Authenticate with Coder deployment                                     |
| [<code>logout</code>](./cli/logout)                 | Unauthenticate your local session                                      |
| [<code>ping</code>](./cli/ping)                     | Ping a workspace                                                       |
| [<code>port-forward</code>](./cli/port-forward)     | Forward ports from machine to a workspace                              |
| [<code>provisionerd</code>](./cli/provisionerd)     | Manage provisioner daemons                                             |
| [<code>publickey</code>](./cli/publickey)           | Output your Coder public key used for Git operations                   |
| [<code>rename</code>](./cli/rename)                 | Rename a workspace                                                     |
| [<code>reset-password</code>](./cli/reset-password) | Directly connect to the database to reset a user's password            |
| [<code>restart</code>](./cli/restart)               | Restart a workspace                                                    |
| [<code>scaletest</code>](./cli/scaletest)           | Run a scale test against the Coder API                                 |
| [<code>schedule</code>](./cli/schedule)             | Schedule automated start and stop times for workspaces                 |
| [<code>server</code>](./cli/server)                 | Start a Coder server                                                   |
| [<code>show</code>](./cli/show)                     | Display details of a workspace's resources and agents                  |
| [<code>speedtest</code>](./cli/speedtest)           | Run upload and download tests from your machine to a workspace         |
| [<code>ssh</code>](./cli/ssh)                       | Start a shell into a workspace                                         |
| [<code>start</code>](./cli/start)                   | Start a workspace                                                      |
| [<code>stat</code>](./cli/stat)                     | Display local system resource usage statistics                         |
| [<code>state</code>](./cli/state)                   | Manually manage Terraform state to fix broken workspaces               |
| [<code>stop</code>](./cli/stop)                     | Stop a workspace                                                       |
| [<code>templates</code>](./cli/templates)           | Manage templates                                                       |
| [<code>tokens</code>](./cli/tokens)                 | Manage personal access tokens                                          |
| [<code>update</code>](./cli/update)                 | Will update and start a given workspace if it is out of date           |
| [<code>users</code>](./cli/users)                   | Manage users                                                           |
| [<code>version</code>](./cli/version)               | Show coder version                                                     |

## Options

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

Additional HTTP headers added to all requests. Provide as key=value. Can be specified multiple times.

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

Specify an authentication token. For security reasons setting CODER_SESSION_TOKEN is preferred.

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
