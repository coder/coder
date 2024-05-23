# Access your workspace

There are many ways to connect to your workspace, the options are only limited by the template configuration.

> Deployment operators can learn more about different types of workspace connections and performance in our [networking docs](../admin/infrastructure/README.md).

You can see the primary methods of connecting to your workspace in the workspace dashboard. 

![Workspace View](../images/user-guides/workspace-view-connection-annotated.png)

<!-- ## Coder Apps

Coder Apps (from our [`coder_app`](https://registry.terraform.io/providers/coder/coder/latest/docs/resources/app) resource in terraform) provide IDE connections and the Terminal. Coder Apps can connect you to ports or run commands on the remote workspace. They be shared with other users by copying the URL from the button. Contact your template administrator if you have IDEs or tools you'd like added into your workspace.

 , and can be extended with our [Module Registry](https://registry.coder.com/modules). -->

## Terminal

The terminal is enabled by implictily in Coder and allows you to access your workspace through a shell environment. 

## Jetbrains IDEs

We support Jetbrains IDEs using Gateway. Currently the following are supported:
  - IntelliJ IDEA
  - CLion
  - GoLand
  - PyCharm
  - Rider
  - RubyMine
  - WebStorm

Read our [docs on Jetbrains Gateway](./jetbrains-gateway.md) for more information on setup.


## Visual Studio Code

Click `VS Code Desktop` in the dashboard to one-click enter a workspace. This
automatically installs the [Coder Remote](https://github.com/coder/vscode-coder)
extension, authenticates with Coder, and connects to the workspace.

![Demo](https://github.com/coder/vscode-coder/raw/main/demo.gif?raw=true)

You can set the default directory in which VS Code opens via the `dir` argument
on the `coder_agent` resource in your workspace template. See the
[Terraform documentation for more details](https://registry.terraform.io/providers/coder/coder/latest/docs/resources/agent#dir).

> The `VS Code Desktop` button can be hidden by enabling
> [Browser-only connections](./networking/index.md#Browser-only).

### Manual Installation

Launch VS Code Quick Open (Ctrl+P), paste the following command, and press
enter.

```text
ext install coder.coder-remote
```

Alternatively, manually install the VSIX from the
[latest release](https://github.com/coder/vscode-coder/releases/latest).

### SSH

### Through with the CLI

Coder will use the optimal path for an SSH connection (determined by your deployment's [networking configuration](../admin/networking.md)) when using the CLI:

```console
coder ssh my-workspace
```

Or, you can configure plain SSH on your client below.


### Configure SSH

Coder generates [SSH key pairs](../secrets.md#ssh-keys) for each user to simplify the setup process.

> Before proceeding, run `coder login <accessURL>` if you haven't already to
> authenticate the CLI with the web UI and your workspaces.

To access Coder via SSH, run the following in the terminal:

```console
coder config-ssh
```

> Run `coder config-ssh --dry-run` if you'd like to see the changes that will be
> made before proceeding.

Confirm that you want to continue by typing **yes** and pressing enter. If
successful, you'll see the following message:

```console
You should now be able to ssh into your workspace.
For example, try running:

$ ssh coder.<workspaceName>
```

Your workspace is now accessible via `ssh coder.<workspace_name>` (e.g.,
`ssh coder.myEnv` if your workspace is named `myEnv`).


## code-server

## Other IDEs
- E

## Ports and Port forwarding

## Other Methods
- 
