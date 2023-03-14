<!-- DO NOT EDIT | GENERATED CONTENT -->
# coder


## Usage
```console
coder [subcommand]
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
| Name |   Purpose |
| ---- |   ----- |
| [<code>dotfiles</code>](./cli/dotfiles) | Checkout and install a dotfiles repository from a Git URL |
| [<code>login</code>](./cli/login) | Authenticate with Coder deployment |
| [<code>logout</code>](./cli/logout) | Unauthenticate your local session |
| [<code>port-forward</code>](./cli/port-forward) | Forward ports from machine to a workspace |
| [<code>publickey</code>](./cli/publickey) | Output your Coder public key used for Git operations |
| [<code>reset-password</code>](./cli/reset-password) | Directly connect to the database to reset a user's password |
| [<code>state</code>](./cli/state) | Manually manage Terraform state to fix broken workspaces |
| [<code>templates</code>](./cli/templates) | Manage templates |
| [<code>users</code>](./cli/users) | Manage users |
| [<code>tokens</code>](./cli/tokens) | Manage personal access tokens |
| [<code>version</code>](./cli/version) | Show coder version |
| [<code>config-ssh</code>](./cli/config-ssh) | Add an SSH Host entry for your workspaces "ssh coder.workspace" |
| [<code>rename</code>](./cli/rename) | Rename a workspace |
| [<code>ping</code>](./cli/ping) | Ping a workspace |
| [<code>create</code>](./cli/create) | Create a workspace |
| [<code>delete</code>](./cli/delete) | Delete a workspace |
| [<code>list</code>](./cli/list) | List workspaces |
| [<code>schedule</code>](./cli/schedule) | Schedule automated start and stop times for workspaces |
| [<code>show</code>](./cli/show) | Display details of a workspace's resources and agents |
| [<code>speedtest</code>](./cli/speedtest) | Run upload and download tests from your machine to a workspace |
| [<code>ssh</code>](./cli/ssh) | Start a shell into a workspace |
| [<code>start</code>](./cli/start) | Start a workspace |
| [<code>stop</code>](./cli/stop) | Stop a workspace |
| [<code>update</code>](./cli/update) | Will update and start a given workspace if it is out of date. |
| [<code>restart</code>](./cli/restart) | Restart a workspace |
| [<code>scaletest</code>](./cli/scaletest) | Run a scale test against the Coder API |
| [<code>server</code>](./cli/server) | Start a Coder server |
| [<code>features</code>](./cli/features) | List Enterprise features |
| [<code>licenses</code>](./cli/licenses) | Add, delete, and list licenses |
| [<code>groups</code>](./cli/groups) | Manage groups |
| [<code>provisionerd</code>](./cli/provisionerd) | Manage provisioner daemons |

## Options
### --url
 
| | |
| --- | --- |
| Environment | <code>$CODER_URL</code> |

URL to a deployment.
### --token
 
| | |
| --- | --- |
| Environment | <code>$CODER_SESSION_TOKEN</code> |

Specify an authentication token. For security reasons setting CODER_SESSION_TOKEN is preferred.
### --agent-token
 
| | |
| --- | --- |

An agent authentication token.
### --agent-url
 
| | |
| --- | --- |
| Environment | <code>$CODER_AGENT_URL</code> |

URL for an agent to access your deployment
### --no-version-warning
 
| | |
| --- | --- |
| Environment | <code>$CODER_NO_VERSION_WARNING</code> |

Suppress warning when client and server versions do not match.
### --no-feature-warning
 
| | |
| --- | --- |
| Environment | <code>$CODER_NO_FEATURE_WARNING</code> |

Suppress warnings about unlicensed features.
### --header
 
| | |
| --- | --- |
| Environment | <code>$CODER_HEADER</code> |

Additional HTTP headers to send to the server.
### --no-open
 
| | |
| --- | --- |
| Environment | <code>$CODER_NO_OPEN</code> |

Suppress opening the browser after logging in.
### --force-tty
 
| | |
| --- | --- |
| Environment | <code>$CODER_FORCE_TTY</code> |

Force the use of a TTY.
### --verbose, -v
 
| | |
| --- | --- |
| Environment | <code>$CODER_VERBOSE</code> |

Enable verbose logging.
### --global-config
 
| | |
| --- | --- |
| Environment | <code>$CODER_CONFIG_DIR</code> |
| Default |     <code>/home/coder/.config/coderv2</code> |



Path to the global `coder` config directory.