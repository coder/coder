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

To access Coder via SSH, run the following in the terminal:

```console
coder config-ssh
```

> Run `coder config-ssh --diff` if you'd like to see the changes that will be
> made before proceeding.

Confirm that you would like to continue by typing **yes** and pressing enter. If
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

1. When prompted, select the **Develop code-server in Docker** option.

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
