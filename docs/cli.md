
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
| [&lt;code&gt;dotfiles&lt;/code&gt;](./cli/dotfiles) | Checkout and install a dotfiles repository from a Git URL |
| [&lt;code&gt;login&lt;/code&gt;](./cli/login) | Authenticate with Coder deployment |
| [&lt;code&gt;logout&lt;/code&gt;](./cli/logout) | Unauthenticate your local session |
| [&lt;code&gt;port-forward&lt;/code&gt;](./cli/port-forward) | Forward ports from machine to a workspace |
| [&lt;code&gt;publickey&lt;/code&gt;](./cli/publickey) | Output your Coder public key used for Git operations |
| [&lt;code&gt;reset-password&lt;/code&gt;](./cli/reset-password) | Directly connect to the database to reset a user&#39;s password |
| [&lt;code&gt;state&lt;/code&gt;](./cli/state) | Manually manage Terraform state to fix broken workspaces |
| [&lt;code&gt;templates&lt;/code&gt;](./cli/templates) | Manage templates |
| [&lt;code&gt;users&lt;/code&gt;](./cli/users) | Manage users |
| [&lt;code&gt;tokens&lt;/code&gt;](./cli/tokens) | Manage personal access tokens |
| [&lt;code&gt;version&lt;/code&gt;](./cli/version) | Show coder version |
| [&lt;code&gt;config-ssh&lt;/code&gt;](./cli/config-ssh) | Add an SSH Host entry for your workspaces &#34;ssh coder.workspace&#34; |
| [&lt;code&gt;rename&lt;/code&gt;](./cli/rename) | Rename a workspace |
| [&lt;code&gt;ping&lt;/code&gt;](./cli/ping) | Ping a workspace |
| [&lt;code&gt;create&lt;/code&gt;](./cli/create) | Create a workspace |
| [&lt;code&gt;delete&lt;/code&gt;](./cli/delete) | Delete a workspace |
| [&lt;code&gt;list&lt;/code&gt;](./cli/list) | List workspaces |
| [&lt;code&gt;schedule&lt;/code&gt;](./cli/schedule) | Schedule automated start and stop times for workspaces |
| [&lt;code&gt;show&lt;/code&gt;](./cli/show) | Display details of a workspace&#39;s resources and agents |
| [&lt;code&gt;speedtest&lt;/code&gt;](./cli/speedtest) | Run upload and download tests from your machine to a workspace |
| [&lt;code&gt;ssh&lt;/code&gt;](./cli/ssh) | Start a shell into a workspace |
| [&lt;code&gt;start&lt;/code&gt;](./cli/start) | Start a workspace |
| [&lt;code&gt;stop&lt;/code&gt;](./cli/stop) | Stop a workspace |
| [&lt;code&gt;update&lt;/code&gt;](./cli/update) | Will update and start a given workspace if it is out of date. |
| [&lt;code&gt;restart&lt;/code&gt;](./cli/restart) | Restart a workspace |
| [&lt;code&gt;scaletest&lt;/code&gt;](./cli/scaletest) | Run a scale test against the Coder API |
| [&lt;code&gt;server&lt;/code&gt;](./cli/server) | Start a Coder server |
| [&lt;code&gt;features&lt;/code&gt;](./cli/features) | List Enterprise features |
| [&lt;code&gt;licenses&lt;/code&gt;](./cli/licenses) | Add, delete, and list licenses |
| [&lt;code&gt;groups&lt;/code&gt;](./cli/groups) | Manage groups |
| [&lt;code&gt;provisionerd&lt;/code&gt;](./cli/provisionerd) | Manage provisioner daemons |

## Options
### --url
URL to a deployment.
<br/>
| | |
| --- | --- |
| Consumes | &lt;code&gt;URL to a deployment.&lt;/code&gt; |

### --token
Specify an authentication token. For security reasons setting CODER_SESSION_TOKEN is preferred.
<br/>
| | |
| --- | --- |
| Consumes | &lt;code&gt;Specify an authentication token. For security reasons setting CODER_SESSION_TOKEN is preferred.&lt;/code&gt; |

### --agent-token
An agent authentication token.
<br/>
| | |
| --- | --- |
| Consumes | &lt;code&gt;An agent authentication token.&lt;/code&gt; |

### --agent-url
URL for an agent to access your deployment
<br/>
| | |
| --- | --- |
| Consumes | &lt;code&gt;URL for an agent to access your deployment&lt;/code&gt; |

### --no-version-warning
Suppress warning when client and server versions do not match.
<br/>
| | |
| --- | --- |
| Consumes | &lt;code&gt;Suppress warning when client and server versions do not match.&lt;/code&gt; |

### --no-feature-warning
Suppress warnings about unlicensed features.
<br/>
| | |
| --- | --- |
| Consumes | &lt;code&gt;Suppress warnings about unlicensed features.&lt;/code&gt; |

### --header
Additional HTTP headers to send to the server.
<br/>
| | |
| --- | --- |
| Consumes | &lt;code&gt;Additional HTTP headers to send to the server.&lt;/code&gt; |

### --no-open
Suppress opening the browser after logging in.
<br/>
| | |
| --- | --- |
| Consumes | &lt;code&gt;Suppress opening the browser after logging in.&lt;/code&gt; |

### --force-tty
Force the use of a TTY.
<br/>
| | |
| --- | --- |
| Consumes | &lt;code&gt;Force the use of a TTY.&lt;/code&gt; |

### --verbose, -v
Enable verbose logging.
<br/>
| | |
| --- | --- |
| Consumes | &lt;code&gt;Enable verbose logging.&lt;/code&gt; |

### --global-config
Path to the global `coder` config directory.
<br/>
| | |
| --- | --- |
| Consumes | &lt;code&gt;Path to the global `coder` config directory.&lt;/code&gt; |
| Default |     &lt;code&gt;/home/coder/.config/coderv2&lt;/code&gt; |


