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

## VS Code in the browser

> You must have Docker Desktop running for this template to work.

Coder offers a [sample template that includes
code-server](../examples/templates/docker-code-server/README.md).

To use:

1. Start Coder:

  ```console
  coder server --dev
  ```

1. Open a new terminal and run:

  ```console
  coder templates init
  ```

1. Select the **Develop code-server in Docker** option when prompted.

1. Navigate into your new folder and create your sample template:

  ```console
  cd code-server-docker && coder templates create
  ```

  Follow the prompts that appear in the terminal.

1. Create your workspace:

  ```console
  coder create --template="docker-code-server" [workspace name]
  ```

1. Log into Coder's Web UI, and open your workspace. Then,
   click **code-server** to launch VS Code in a new browser window.

## JetBrains Gateway with SSH

If your image
[includes a JetBrains IDE](../admin/workspace-management/installing-jetbrains.md)
and you've [set up SSH access to Coder](./ssh.md), you can use JetBrains Gateway
to run a local JetBrains IDE connected to your Coder workspace.

> See the [Docker sample template](../examples/templates/docker/main.tf) for an
> example of how to refer to images in your template.

Please note that:

- Your Coder workspace must be running, and Gateway needs compute resources, so
  monitor your resource usage on the Coder dashboard and adjust accordingly.
- If you use a premium JetBrains IDE (e.g., GoLand, IntelliJ IDEA Ultimate), you
  will still need a license to use it remotely with Coder.

1. [Download and install JetBrains Toolbox](https://www.jetbrains.com/toolbox-app/).
   Locate JetBrains Gateway in the Toolbox list and click **Install**.

1. Open JetBrains Gateway and click **Connect via SSH** within the **Run the IDE
   Remotely** section.

1. Click the small **gear icon** to the right of the **Connection** field, then
   the **+** button on the next screen to create a new configuration.

1. Enter your Coder workspace alias target in **Host** (e.g.,
   `coder.<yourWorkspaceName>`), `22` in **Port**, `coder` in **User name**, and change
   **Authentication Type** to **OpenSSH config and authentication agent**. Leave
   the local port field blank. Click **Test Connection**. If the test is
   successful, click **Ok** at the bottom to proceed.

1. With your created configuration in the **Connection** chosen in the drop-down
   field, click **Check Connection and Continue**.

1. Select a JetBrains IDE from the **IDE version** drop-down (make sure that you
   choose the IDE included in your image), then click the folder icon and select the
   `/home/coder` directory in your Coder workspace. Click **Download and Start
   IDE** to proceed.

1. During this installation step, Gateway downloads the IDE and a JetBrains
   client. This may take a couple of minutes.

1. When your IDE download is complete, JetBrains will prompt you for your
   license. When done, you'll be able to use your IDE.
