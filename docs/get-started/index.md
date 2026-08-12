# Quickstart

Follow this guide to get your first Coder development environment
running in under 10 minutes. This guide covers the essential concepts and shows
you how to create your first workspace and open it in your preferred editor.
This workspace includes a basic set of tools to edit most code bases.

## What you'll do

In this quickstart, you'll:

- ✅ Install Coder server.
- ✅ Create a **template** (blueprint for dev environments).
- ✅ Launch a **workspace** (your actual dev environment).
- ✅ Connect from your favorite IDE.

## A 30-second metaphor for Coder

Before diving in, the following table breaks down the core concepts that power Coder,
explained through a cooking analogy:

| Component      | What It Is                                                                           | Real-World Analogy             |
|----------------|--------------------------------------------------------------------------------------|--------------------------------|
| **You**        | The engineer/developer/builder working                                               | The head chef cooking the meal |
| **Templates**  | A Terraform blueprint that defines your dev environment (OS, tools, resources)       | Recipe for a meal              |
| **Workspaces** | The actual running environment created from the template                             | The cooked meal                |
| **Users**      | A developer who launches the workspace from a template and does their work inside it | The people eating the meal     |

**Putting it Together:** Coder separates who _defines_ environments from who _uses_ them. Admins create and manage Templates, the recipes, while developers use those Templates to launch Workspaces, the meals.

## Prerequisites

- A machine with 2+ CPU cores and 4GB+ RAM
- Familiarity with running commands in the terminal
- 10 minutes of your time

> [!TIP]
> If you use a coding agent like Claude Code, the [coder/skills](https://github.com/coder/skills) `setup` skill can train the coding agent on the following steps (install a container runtime, install Coder, create your first template, and launch a workspace).

## Step 1: Install a container runtime

Coder needs a Docker-compatible container runtime running on the host, such as
[Colima](https://colima.run), [Rancher Desktop](https://rancherdesktop.io),
[Podman](https://podman.io), or
[Docker Desktop](https://www.docker.com/products/docker-desktop/). If you
already have one installed and running, skip ahead to
[Step 2](#step-2-install-and-start-coder). Otherwise, follow the steps below to
install a free runtime quickly on your platform.

<div class="tabs">

### Linux

1. Install Docker Engine:

   ```sh
   curl -sSL https://get.docker.com | sh
   ```

   For more details, visit [Docker's docs on installing Docker on Linux](https://docs.docker.com/desktop/install/linux-install/).

1. Assign your user to the Docker group:

   ```sh
   sudo usermod -aG docker $USER
   ```

1. Run `newgrp` to activate the groups changes:

   ```sh
   newgrp docker
   ```

   You might need to log out of and back into your machine or restart your
   machine for changes to take effect.

1. Launch the Docker daemon:

   ```sh
   sudo systemctl start docker
   ```

### macOS

[Colima](https://colima.run) is a free, lightweight container runtime that
provides the Docker daemon on macOS without the overhead of Docker Desktop.

1. Install Colima and the Docker CLI with [Homebrew](https://brew.sh):

   ```sh
   brew install colima docker
   ```

1. Start Colima to launch the Docker daemon:

   ```sh
   colima start
   ```

1. Point `DOCKER_HOST` at the Colima socket so Coder can reach the daemon:

   ```sh
   export DOCKER_HOST="unix://${HOME}/.colima/default/docker.sock"
   ```

   Colima exposes its Docker socket at `~/.colima/default/docker.sock`, not `/var/run/docker.sock`, so Coder needs `DOCKER_HOST` to find it.
   Set it in the same terminal where you'll run `coder server` in the next step.
   To persist it across new terminals and restarts, add the `export` line to your shell's startup file, such as `~/.zshrc`, `~/.bashrc`, or `~/.config/fish/config.fish`.

### Windows

If you plan to use the built-in PostgreSQL database, ensure that the
[Visual C++ Runtime](https://learn.microsoft.com/en-US/cpp/windows/latest-supported-vc-redist#latest-microsoft-visual-c-redistributable-version)
is installed.

[Podman Desktop](https://podman-desktop.io) is a free GUI for the Podman container runtime.
Its onboarding installs and configures the required
Windows Subsystem for Linux (WSL2) or Hyper-V layer if it isn't already enabled.

1. Download and install [Podman Desktop](https://podman-desktop.io/downloads).

1. Follow the onboarding to configure Podman.

1. If you configured Podman to use WSL2, then you will need to do either
   upgrade WSL2 to version 2.5.1 or later
   (which uses [cgroups](https://wikipedia.org/wiki/Cgroups) v2 by default)
   or create a `.wslconfig` file in the `%USERPROFILE%` directory
   with the following contents

   ```txt
   [wsl2]
   kernelCommandLine=cgroup_no_v1=all
   ```

   This is not required for Podman with Hyper-V.

1. Open Podman Desktop and complete the onboarding to create and start a
   Podman machine.

   Podman Desktop enables Docker socket compatibility by default, so tools
   that expect the Docker daemon work without additional configuration.

</div>

## Step 2: Install and start Coder

Install the `coder` CLI to get started:

<div class="tabs">

### Linux/macOS

1. Install Coder:

   ```sh
   curl -L https://coder.com/install.sh | sh
   ```

   - For standalone binaries, system packages, or other alternate installation
     methods, refer to the
     [latest release on GitHub](https://github.com/coder/coder/releases/latest).

1. Start Coder:

   ```sh
   coder server
   ```

### Windows

If you plan to use the built-in PostgreSQL database, ensure that the
[Visual C++ Runtime](https://learn.microsoft.com/en-US/cpp/windows/latest-supported-vc-redist#latest-microsoft-visual-c-redistributable-version)
is installed.

1. Use the
   [`winget`](https://learn.microsoft.com/en-us/windows/package-manager/winget/#use-winget)
   package manager to install Coder:

   ```ps1
   winget install Coder.Coder
   ```

1. Start Coder:

   ```sh
   coder server
   ```

</div>

Coder will attempt to open the setup page in your browser. If it doesn't open
automatically, go to <http://localhost:3000>.

- If you get a browser warning similar to `Secure Site Not Available`, you can
  ignore the warning and continue to the setup page.

If your Coder server is on a network or cloud device, or you are having trouble
viewing the page, locate the web UI URL in Coder logs in your terminal. It looks
like `https://<CUSTOM-STRING>.<TUNNEL>.try.coder.app`. It's one of the first
lines of output, so you might have to scroll up to find it.

This section includes some of the quicker ways to install Coder. For more detailed options, visit the [Install](../install/index.md) page.

## Step 3: Initial setup

1. Create your admin account:
   - Email: `your.email@example.com`
   - Password: Choose a strong password.

   You can also choose to **Continue with GitHub** instead of creating an admin
   account. Coder automatically grants admin permissions to the first user that signs in.

   ![Welcome to Coder - Create admin user](../images/screenshots/welcome-create-admin-user.png)

## Step 4: Create your first template and workspace

> [!TIP]
> If you use an AI coding assistant, the [coder-templates](https://github.com/coder/registry/blob/main/.agents/skills/coder-templates/SKILL.md) agent skill can guide you through creating and customizing templates with best practices built-in.

Templates define what's in your development environment. The template builder
guides you through creating one without writing any Terraform.

1. Select **Templates** > **New Template**. The template builder opens.

1. Select the **Docker** base template from the list.

   > [!NOTE]
   > This template requires Docker to be running in the background, so make sure Docker is running.

1. Skip or configure any base template parameters, then select modules to add
   IDEs and tools to your template. For example, add **code-server** to get
   VS Code in the browser. You can skip module selection for now and add
   modules later.

1. On the final step, name your template:
   - **Name**: `my-docker-template`
   - **Display name** and **Description**: fill in as you like.

1. Select **Create Template**. Coder composes and validates the Terraform
   configuration, then creates your template.

   ![Template builder base selection step](../images/templatebuilder_01_bases.png)

**What just happened?**
The template builder selected a base infrastructure template, composed it with
any modules you chose, and generated a valid Terraform configuration. Coder
validated the configuration server-side, then created a reusable template in
your organization's template list. You and any teammates in the same
organization can now create workspaces from it.

<details>
<summary>What happens under the hood?</summary>

A Coder template is a [Terraform](https://developer.hashicorp.com/terraform/intro) configuration, and Coder is built on top of Terraform.
When you create a workspace from this template, a Coder [provisioner](../admin/infrastructure/architecture.md#provisionerd) runs a Terraform job from the template's configuration to build your environment.
For the Docker base template, that job starts a Docker container with the Coder agent pre-configured, along with any modules you selected.

To learn how Coder uses Terraform to provision and run workspaces, refer to the [architecture overview](../admin/infrastructure/architecture.md).

</details>

Now it's time to launch a workspace.

## Step 5: Launch your workspace

1. After the template is ready, select **+ Create Workspace**.

1. Give the workspace a name. If you need a suggestion, you can select the
   automatically generated name next to the **Need a suggestion?** label.

1. If the template has any
   [parameters](../admin/templates/extending-templates/parameters.md), fill
   them in. Parameters vary by template and the modules you selected in the
   builder.

1. Select **Create workspace**.

After a short wait (10-15 seconds on most modern computers), Coder will start your new workspace:

![getting-started-workspace is running](../images/screenshots/workspace-running-with-topbar.png)_Workspace is running_

## Step 6: Connect your IDE

Each button in the workspace view is a different **agent app**.
The buttons in the UI reflect the modules you added in the template builder (such as code-server, Claude Code, or any of the JetBrains editors).
Select your preferred IDE from the list of agent apps.

This guide uses **VS Code Desktop**, which opens the workspace in the VS Code installed on your local machine, using the Coder extension.
To stay in the browser instead, select the browser-based VS Code option.
The remaining steps are similar for other IDEs and editors.

After VS Code loads the remote environment, you can select **Open Folder** to
explore directories in the Docker container or work on something new.

![Changing directories in VS Code](../images/screenshots/change-directory-vscode.png)

If you didn't clone an existing Git repository when you created your
workspace, you can clone it manually if you want:

1. Select **Clone Repository** and enter the repository URL.

   For example, to clone the Coder repo, enter
   `https://github.com/coder/coder.git`.

   Learn more about how to find the repository URL in the
   [GitHub documentation](https://docs.github.com/en/repositories/creating-and-managing-repositories/cloning-a-repository).

2. Choose the folder to which VS Code should clone the repo. It will be in its
   own directory within this folder.

   Note that you cannot create a new parent directory in this step.

3. After VS Code completes the clone, select **Open** to open the directory.

4. You are now using VS Code in your Coder environment!

## What's next?

You now have:

- A Coder server running locally.
- A template defining your environment.
- A workspace running that environment.
- IDE access to code remotely.

Now that you have your own workspace running, you can [customize your template](./customize-your-template/index.md) to fit your needs.

## Learn more

- [Try Coder Agents](../ai-coder/agents/getting-started.md), the chat
  interface and API for delegating development work to coding agents in your
  Coder deployment.

- [Read about managing Workspaces for your team](../user-guides/workspace-management.md)

- [Read about implementing monitoring tools for your Coder Deployment](../admin/monitoring/index.md)

## Troubleshooting

### Cannot connect to the Docker daemon

When creating a workspace from a Docker template, you may see an error like:

```txt
Error: Error pinging Docker server: Cannot connect to the Docker daemon at unix:///var/run/docker.sock. Is the docker daemon running?
```

This usually means a container runtime is either not installed or not running on the machine where Coder is running.
A runtime must be running before you create a workspace from a Docker-based template.

If the runtime is running but Coder still cannot connect, the daemon may expose its socket at a path other than `/var/run/docker.sock`.
This is common with Colima on macOS and with rootless Docker on Linux.
In that case, point Coder at the socket with the `DOCKER_HOST` environment variable, then restart the Coder server.

<div class="tabs">

#### Linux

1. Install Docker, if you haven't already:

   ```sh
   curl -sSL https://get.docker.com | sh
   ```

1. Start the Docker daemon:

   ```sh
   sudo systemctl start docker
   ```

1. Assign your user to the `docker` group so Coder can access the daemon
   without root:

   ```sh
   sudo usermod -aG docker $USER
   newgrp docker
   ```

1. Confirm the group membership:

   ```console
   $ groups
   docker sudo users
   ```

#### macOS

1. If Colima is not installed, install it with [Homebrew](https://brew.sh):

   ```sh
   brew install colima docker
   ```

1. Start Colima to launch the Docker daemon:

   ```sh
   colima start
   ```

1. Verify that the daemon is reachable:

   ```sh
   docker ps
   ```

1. If `docker ps` works but Coder still cannot connect, point `DOCKER_HOST` at the Colima socket, then restart the Coder server:

   ```sh
   export DOCKER_HOST="unix://${HOME}/.colima/default/docker.sock"
   ```

   To persist the setting across restarts, add that `export` line to your shell's startup file, such as `~/.zshrc`, `~/.bashrc`, or `~/.config/fish/config.fish`.

#### Windows

1. If Podman Desktop is not installed,
   [download and install it](https://podman-desktop.io/downloads).

1. Open Podman Desktop and verify that a Podman machine is running.

</div>

### Can't start Coder server: Address already in use

```txt
Encountered an error running "coder server", see "coder server --help" for more information
error: configure http(s): listen tcp 127.0.0.1:3000: bind: address already in use
```

Another process is already listening on port 3000. Identify and stop it,
then start the server again.

<div class="tabs">

#### Linux

1. Stop the process:

   ```sh
   sudo systemctl stop coder
   ```

1. Start Coder:

   ```sh
   coder server
   ```

#### macOS

1. Identify the process using port 3000:

   ```sh
   lsof -i :3000
   ```

1. Stop the process using the PID from the previous command:

   ```sh
   kill <PID>
   ```

   If the process does not exit, force-kill it:

   ```sh
   kill -9 <PID>
   ```

1. Start Coder:

   ```sh
   coder server
   ```

#### Windows

1. Identify the process using port 3000 in PowerShell:

   ```ps1
   Get-NetTCPConnection -LocalPort 3000 | Select-Object OwningProcess
   ```

1. Stop the process using the PID from the previous command:

   ```ps1
   Stop-Process -Id <PID>
   ```

1. Start Coder:

   ```sh
   coder server
   ```

</div>
