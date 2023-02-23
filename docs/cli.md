<!-- DO NOT EDIT | GENERATED CONTENT -->
# coder


Coder v0.0.0-devel — A tool for provisioning self-hosted development environments with Terraform.


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
| Name |   Purpose |
| ---- |   ----- |
| <code>config-ssh</code> | Add an SSH Host entry for your workspaces "ssh coder.workspace" |
| <code>create</code> | Create a workspace |
| <code>delete</code> | Delete a workspace |
| <code>dotfiles</code> | Checkout and install a dotfiles repository from a Git URL |
| <code>list</code> | List workspaces |
| <code>login</code> | Authenticate with Coder deployment |
| <code>logout</code> | Unauthenticate your local session |
| <code>ping</code> | Ping a workspace |
| <code>port-forward</code> | Forward ports from machine to a workspace |
| <code>publickey</code> | Output your Coder public key used for Git operations |
| <code>rename</code> | Rename a workspace |
| <code>reset-password</code> | Directly connect to the database to reset a user's password |
| <code>restart</code> | Restart a workspace |
| <code>scaletest</code> | Run a scale test against the Coder API |
| <code>schedule</code> | Schedule automated start and stop times for workspaces |
| <code>server</code> | Start a Coder server |
| <code>show</code> | Display details of a workspace's resources and agents |
| <code>speedtest</code> | Run upload and download tests from your machine to a workspace |
| <code>ssh</code> | Start a shell into a workspace |
| <code>start</code> | Start a workspace |
| <code>state</code> | Manually manage Terraform state to fix broken workspaces |
| <code>stop</code> | Stop a workspace |
| <code>templates</code> | Manage templates |
| <code>tokens</code> | Manage personal access tokens |
| <code>update</code> | Update a workspace |
| <code>users</code> | Manage users |
| <code>version</code> | Show coder version |
