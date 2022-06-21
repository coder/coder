# IDEs

The following desktop IDEs have been tested with Coder, though any IDE with SSH
support should work:

- VS Code (with [Remote -
  SSH](https://marketplace.visualstudio.com/items?itemName=ms-vscode-remote.remote-ssh)
  extension)
- JetBrains (with
  [Gateway](https://www.jetbrains.com/help/idea/remote-development-a.html#launch_gateway)
  installed)
  - IntelliJ IDEA
  - CLion
  - GoLand
  - PyCharm
  - Rider
  - RubyMine
  - WebStorm
- Web IDEs (code-server, JupyterLab, Jetbrains Projector)
   - Note: These are [configured in the template](./ides/configuring-web-ides.md)

## SSH configuration

> Before proceeding, run `coder login <accessURL>` if you haven't already to
> authenticate the CLI with the web UI and your workspaces.

To access Coder via SSH, run the following in the terminal:

```console
coder config-ssh
```

> Run `coder config-ssh --diff` if you'd like to see the changes that will be
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

1. Install the [Remote - SSH](https://marketplace.visualstudio.com/items?itemName=ms-vscode-remote.remote-ssh)
   extension.

1. In VS Code's left-hand nav bar, click **Remote Explorer** and right-click on
   a workspace to connect.

## Web IDEs (Jupyter, code-server, Jetbrains Projector)

Web IDEs (code-server, JetBrains Projector, VNC, etc.) are defined in the template. See [configuring IDEs](./ides/configuring-web-ides.md).
