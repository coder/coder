# IDEs

The following desktop IDEs have been tested with Coder, though any IDE with SSH
support should work:

- [VS Code Remote SSH](#vs-code-remote)
- [JetBrains with Gateway](./ides/gateway.md)
  - IntelliJ IDEA
  - CLion
  - GoLand
  - PyCharm
  - Rider
  - RubyMine
  - WebStorm
- Web IDEs (code-server, JupyterLab, JetBrains Projector)
  - Note: These are [configured in the template](./ides/web-ides.md)
- [Emacs](./ides/emacs-tramp.md)

## SSH configuration

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

## VS Code Remote

Once you've configured SSH, you can work on projects from your local copy of VS
Code, connected to your Coder workspace for compute, etc.

1. Open VS Code locally.

1. Install the [Remote -
   SSH](https://marketplace.visualstudio.com/items?itemName=ms-vscode-remote.remote-ssh)
   extension.

1. In VS Code's left-hand nav bar, click **Remote Explorer** and right-click on
   a workspace to connect.

## JetBrains Gateway

Gateway operates in a client-server model, using an SSH connection to the remote
host to install and start the server.

Setting up Gateway also involves picking a project directory, so if you have not
already done so, you may wish to open a terminal on your Coder workspace and
check out a copy of the project you intend to work on.

After installing Gateway on your local system, [follow these steps to create a
Connection and connect to your Coder workspace.](./ides/gateway.md)

| Version   | Status  | Notes                                                    |
| --------- | ------- | -------------------------------------------------------- |
| 2021.3.2  | Working |                                                          |
| 2022.1.4  | Working | Windows clients are unable to connect to Linux workspace |
| 2022.2 RC | Working | Version >= 222.3345.108                                  |

## Web IDEs (Jupyter, code-server, JetBrains Projector)

Web IDEs (code-server, JetBrains Projector, VNC, etc.) are defined in the template. See [IDEs](./ides/web-ides.md).

## Up next

- Learn about [Port Forwarding](./networking/port-forwarding.md)
