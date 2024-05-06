# IDEs

The following desktop IDEs have been tested with Coder, though any IDE with SSH
support should work:

- [Visual Studio Code](#visual-studio-code)
- [JetBrains with Gateway](./ides/gateway.md)
  - IntelliJ IDEA
  - CLion
  - GoLand
  - PyCharm
  - Rider
  - RubyMine
  - WebStorm
- [JetBrains Fleet](./ides/fleet.md)
- Web IDEs (code-server, JupyterLab, JetBrains Projector)
  - Note: These are [configured in the template](./ides/web-ides.md)
- [Emacs](./ides/emacs-tramp.md)

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

## SSH configuration

> Before proceeding, run `coder login <accessURL>` if you haven't already to
> authenticate the CLI with the web UI and your workspaces.

To access Coder via SSH, run the following in the terminal:

```shell
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

## JetBrains Gateway

Gateway operates in a client-server model, using an SSH connection to the remote
host to install and start the server.

Setting up Gateway also involves picking a project directory, so if you have not
already done so, you may wish to open a terminal on your Coder workspace and
check out a copy of the project you intend to work on.

After installing Gateway on your local system,
[follow these steps to create a Connection and connect to your Coder workspace.](./ides/gateway.md)

| Version   | Status  | Notes                                                    |
| --------- | ------- | -------------------------------------------------------- |
| 2021.3.2  | Working |                                                          |
| 2022.1.4  | Working | Windows clients are unable to connect to Linux workspace |
| 2022.2 RC | Working | Version >= 222.3345.108                                  |

## Web IDEs (Jupyter, code-server, JetBrains Projector)

Web IDEs (code-server, JetBrains Projector, VNC, etc.) are defined in the
template. See [IDEs](./ides/web-ides.md).

## Up next

- Learn about [Port Forwarding](./networking/port-forwarding.md)
