<!-- DO NOT EDIT | GENERATED CONTENT -->

# coder

Coder — A tool for provisioning self-hosted development environments with Terraform.

## Usage

```console
coder [flags]
```

## Examples

```console
  - Start a Coder server:

      $ coder server

  - Get started by creating a template from an example:

      $ coder templates init
```

## Subcommands

| Name                                                      | Purpose                                                         |
| --------------------------------------------------------- | --------------------------------------------------------------- |
| [<code>config-ssh</code>](./cli/coder_config-ssh)         | Add an SSH Host entry for your workspaces "ssh coder.workspace" |
| [<code>create</code>](./cli/coder_create)                 | Create a workspace                                              |
| [<code>delete</code>](./cli/coder_delete)                 | Delete a workspace                                              |
| [<code>dotfiles</code>](./cli/coder_dotfiles)             | Checkout and install a dotfiles repository from a Git URL       |
| [<code>list</code>](./cli/coder_list)                     | List workspaces                                                 |
| [<code>login</code>](./cli/coder_login)                   | Authenticate with Coder deployment                              |
| [<code>logout</code>](./cli/coder_logout)                 | Unauthenticate your local session                               |
| [<code>ping</code>](./cli/coder_ping)                     | Ping a workspace                                                |
| [<code>port-forward</code>](./cli/coder_port-forward)     | Forward ports from machine to a workspace                       |
| [<code>publickey</code>](./cli/coder_publickey)           | Output your Coder public key used for Git operations            |
| [<code>rename</code>](./cli/coder_rename)                 | Rename a workspace                                              |
| [<code>reset-password</code>](./cli/coder_reset-password) | Directly connect to the database to reset a user's password     |
| [<code>restart</code>](./cli/coder_restart)               | Restart a workspace                                             |
| [<code>scaletest</code>](./cli/coder_scaletest)           | Run a scale test against the Coder API                          |
| [<code>schedule</code>](./cli/coder_schedule)             | Schedule automated start and stop times for workspaces          |
| [<code>server</code>](./cli/coder_server)                 | Start a Coder server                                            |
| [<code>show</code>](./cli/coder_show)                     | Display details of a workspace's resources and agents           |
| [<code>speedtest</code>](./cli/coder_speedtest)           | Run upload and download tests from your machine to a workspace  |
| [<code>ssh</code>](./cli/coder_ssh)                       | Start a shell into a workspace                                  |
| [<code>start</code>](./cli/coder_start)                   | Start a workspace                                               |
| [<code>state</code>](./cli/coder_state)                   | Manually manage Terraform state to fix broken workspaces        |
| [<code>stop</code>](./cli/coder_stop)                     | Stop a workspace                                                |
| [<code>templates</code>](./cli/coder_templates)           | Manage templates                                                |
| [<code>tokens</code>](./cli/coder_tokens)                 | Manage personal access tokens                                   |
| [<code>update</code>](./cli/coder_update)                 | Update a workspace                                              |
| [<code>users</code>](./cli/coder_users)                   | Manage users                                                    |
| [<code>version</code>](./cli/coder_version)               | Show coder version                                              |
