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

1. Install the [Remote - SSH](https://marketplace.visualstudio.com/items?itemName=ms-vscode-remote.remote-ssh)
   extension.

1. In VS Code's left-hand nav bar, click **Remote Explorer** and right-click on
   a workspace to connect.

## JetBrains Gateway

Gateway operates in a client-server model, using an SSH connection to the remote host to install
and start the server.

Setting up Gateway also involves picking a project directory, so if you have not already done so,
you may wish to open a terminal on your Coder workspace and check out a copy of the project you
intend to work on.

After installing Gateway on your local system, you may connect to a Coder workspace as follows

1. Open Gateway, make sure "SSH" is selected under "Remote Development"
2. Click "New Connection"
3. In the resulting dialog, click the gear icon to the right of "Connection:"
4. Hit the "+" button to add a new SSH connection
   1. For the Host, enter `coder.<workspace name>`
   2. For the Port, enter `22` (this is ignored by Coder)
   3. For the Username, enter `coder`
   4. For the Authentication Type, select "OpenSSH config and authentication agent"
   5. Make sure the checkbox for "Parse config file ~/.ssh/config" is checked.
   6. Click "Test Connection" to ensure you setting are ok.
   7. Click "OK"
5. Select the connection you just added.
6. Click "Check Connection and Continue"
7. Select the JetBrains IDE for your project and the project directory
   1. Use an SSH terminal to your workspace to create a directory or check out code if you haven't
      already.
8. Click "Download and Start IDE" to connect.

| Version   | Status      | Notes                                                      |
|-----------|-------------|------------------------------------------------------------|
| 2021.3.2  | Working     |                                                            |
| 2022.1.1  | Working     | Windows clients are unable to connect to Linux workspace   |
| 2022.2 RC | Not working | [GitHub Issue](https://github.com/coder/coder/issues/3125) |


## Web IDEs (Jupyter, code-server, Jetbrains Projector)

Web IDEs (code-server, JetBrains Projector, VNC, etc.) are defined in the template. See [configuring IDEs](./ides/configuring-web-ides.md).
