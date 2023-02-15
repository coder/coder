## coder

### Synopsis

Coder — A tool for provisioning self-hosted development environments with Terraform.

```
coder [flags]
```

### Examples

```
  - Start a Coder server:

      $ coder server

  - Get started by creating a template from an example:

      $ coder templates init
```

### Options

```
      --global-config coder   Path to the global coder config directory.
                              Consumes $CODER_CONFIG_DIR (default "~/.config/coderv2")
      --header stringArray    HTTP headers added to all requests. Provide as "Key=Value".
                              Consumes $CODER_HEADER
  -h, --help                  help for coder
      --no-feature-warning    Suppress warnings about unlicensed features.
                              Consumes $CODER_NO_FEATURE_WARNING
      --no-version-warning    Suppress warning when client and server versions do not match.
                              Consumes $CODER_NO_VERSION_WARNING
      --token string          Specify an authentication token. For security reasons setting CODER_SESSION_TOKEN is preferred.
                              Consumes $CODER_SESSION_TOKEN
      --url string            URL to a deployment.
                              Consumes $CODER_URL
  -v, --verbose               Enable verbose output.
                              Consumes $CODER_VERBOSE
```

### SEE ALSO

- [coder config-ssh](coder_config-ssh.md) - Add an SSH Host entry for your workspaces "ssh coder.workspace"
- [coder create](coder_create.md) - Create a workspace
- [coder delete](coder_delete.md) - Delete a workspace
- [coder dotfiles](coder_dotfiles.md) - Checkout and install a dotfiles repository from a Git URL
- [coder list](coder_list.md) - List workspaces
- [coder login](coder_login.md) - Authenticate with Coder deployment
- [coder logout](coder_logout.md) - Unauthenticate your local session
- [coder ping](coder_ping.md) - Ping a workspace
- [coder port-forward](coder_port-forward.md) - Forward ports from machine to a workspace
- [coder publickey](coder_publickey.md) - Output your Coder public key used for Git operations
- [coder rename](coder_rename.md) - Rename a workspace
- [coder reset-password](coder_reset-password.md) - Directly connect to the database to reset a user's password
- [coder restart](coder_restart.md) - Restart a workspace
- [coder scaletest](coder_scaletest.md) - Run a scale test against the Coder API
- [coder schedule](coder_schedule.md) - Schedule automated start and stop times for workspaces
- [coder server](coder_server.md) - Start a Coder server
- [coder show](coder_show.md) - Display details of a workspace's resources and agents
- [coder speedtest](coder_speedtest.md) - Run upload and download tests from your machine to a workspace
- [coder ssh](coder_ssh.md) - Start a shell into a workspace
- [coder start](coder_start.md) - Start a workspace
- [coder state](coder_state.md) - Manually manage Terraform state to fix broken workspaces
- [coder stop](coder_stop.md) - Stop a workspace
- [coder templates](coder_templates.md) - Manage templates
- [coder tokens](coder_tokens.md) - Manage personal access tokens
- [coder update](coder_update.md) - Update a workspace
- [coder users](coder_users.md) - Manage users
- [coder version](coder_version.md) - Show coder version
