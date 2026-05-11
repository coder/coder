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

## Step 1: Install Docker and set up permissions

<div class="tabs">

### Linux

1. Install Docker:

   ```bash
   curl -sSL https://get.docker.com | sh
   ```

   For more details, visit [Docker's docs on installing Docker on Linux](https://docs.docker.com/desktop/install/linux-install/).

1. Assign your user to the Docker group:

   ```shell
   sudo usermod -aG docker $USER
   ```

1. Run `newgrp` to activate the groups changes:

   ```shell
   newgrp docker
   ```

   You might need to log out of and back into your machine or restart your
   machine for changes to take effect.

1. Launch the Docker daemon:

   ```shell
   sudo systemctl start docker
   ```

### macOS

1. [Install Docker](https://docs.docker.com/desktop/setup/install/mac-install/).
There is a Homebrew formula for the Docker command and a Homebrew cask of Docker
Desktop if you prefer:

   ```shell
   brew install --cask docker-desktop
   ```

1. Open Docker Desktop.

### Windows

If you plan to use the built-in PostgreSQL database, ensure that the
[Visual C++ Runtime](https://learn.microsoft.com/en-US/cpp/windows/latest-supported-vc-redist#latest-microsoft-visual-c-redistributable-version)
is installed.

1. [Install Docker](https://docs.docker.com/desktop/install/windows-install/).

1. Open Docker Desktop.

</div>

## Step 2: Install and start Coder

Install the `coder` CLI to get started:

<div class="tabs">

### Linux/macOS

1. Install Coder:

   ```shell
   curl -L https://coder.com/install.sh | sh
   ```

   - For standalone binaries, system packages, or other alternate installation
     methods, refer to the
     [latest release on GitHub](https://github.com/coder/coder/releases/latest).

1. Start Coder:

   ```shell
   coder server
   ```

### Windows

If you plan to use the built-in PostgreSQL database, ensure that the
[Visual C++ Runtime](https://learn.microsoft.com/en-US/cpp/windows/latest-supported-vc-redist#latest-microsoft-visual-c-redistributable-version)
is installed.

1. Use the
   [`winget`](https://learn.microsoft.com/en-us/windows/package-manager/winget/#use-winget)
   package manager to install Coder:

   ```powershell
   winget install Coder.Coder
   ```

1. Start Coder:

   ```shell
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

Templates define what's in your development environment. The following is a basic example:

1. Select **Templates** → **New Template**.

2. Select the **Coder Quickstart** template from the list of starter templates.

   **Note:** running this template requires Docker to be running in the background, so make sure Docker is running!

3. Name your template:
   - Name: `quickstart`
   - Display name: `quickstart doc template`
   - Description: `Provision Docker containers as Coder workspaces`

4. Select **Save**.

   ![Create template](../images/screenshots/create-template.png)

**What just happened?** You defined a template — a reusable blueprint for dev
environments — in your Coder deployment. It's now stored in your organization's
template list, where you and any teammates in the same org can create workspaces
from it. Now it's time launch a workspace.

## Step 5: Launch your workspace

1. After the template is ready, select **+ Create Workspace**.

2. Give the workspace a name. If you need a suggestion for a workspace, you can select the automatically generated name next to the **Need a suggestion?** label.

3. In this window are [parameters](../admin/templates/extending-templates/parameters.md) that customize the workspace's behavior. Set the following based on your needs:

   - **Programming Languages**: the languages to pre-install in your workspace. You can use more than one if you want.
   - **IDEs & Editors**: the IDEs and editors you want to configure for quick access once the workspace is running. You can choose more than one if you want.
   - **Git Repository (Optional)**: the Git repository you want to clone into your workspace. Leave this field blank to skip it.

   **Note:** If you use any of the JetBrains IDEs as your preferred IDE (such as PyCharm, GoLand, or RustRover), select **JetBrains IDEs** as the value. A new parameter will appear, with which you can choose your preferred JetBrains IDE.

4. Launch your workspace by selecting **Create workspace**.

After a short wait (10-15 seconds on most modern computers), Coder will start your new workspace:

![getting-started-workspace is running](../images/screenshots/workspace-running-with-topbar.png)_Workspace is running_

## Step 6: Connect your IDE

Each of the buttons in the workspace view is a different **agent app**
(more on this in a later section). Select your preferred IDE from the
list of agent apps. This guide assumes you'll use Visual Studio Code,
but the process is similar for other IDEs and editors.

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

## Success! You're coding in Coder

You now have:

- A Coder server running locally.
- A template defining your environment.
- A workspace running that environment.
- IDE access to code remotely.

### What's next?

Now that you have your own workspace running, you can start exploring more
advanced capabilities that Coder offers.

- [Try Coder Agents](../ai-coder/agents/getting-started.md), the chat
  interface and API for delegating development work to coding agents in your
  Coder deployment.

- [Read about managing Workspaces for your team](../user-guides/workspace-management.md)

- [Read about implementing monitoring tools for your Coder Deployment](../admin/monitoring/index.md)

## Troubleshooting

### Cannot connect to the Docker daemon

When creating a workspace from a Docker template, you may see an error like:

```text
Error: Error pinging Docker server: Cannot connect to the Docker daemon at unix:///var/run/docker.sock. Is the docker daemon running?
```

This means Docker is either not installed or not running on the machine where
Coder is running. Docker must be running before you create a workspace from a
Docker-based template.

<div class="tabs">

#### macOS

1. If Docker Desktop is not installed,
   [install it](https://docs.docker.com/desktop/setup/install/mac-install/) or
   use Homebrew:

   ```shell
   brew install --cask docker-desktop
   ```

1. Open Docker Desktop and verify that it is running.

#### Linux

1. Install Docker, if you haven't already:

   ```shell
   curl -sSL https://get.docker.com | sh
   ```

1. Start the Docker daemon:

   ```shell
   sudo systemctl start docker
   ```

1. Assign your user to the `docker` group so Coder can access the daemon
   without root:

   ```shell
   sudo usermod -aG docker $USER
   newgrp docker
   ```

1. Confirm the group membership:

   ```console
   $ groups
   docker sudo users
   ```

#### Windows

1. If Docker Desktop is not installed,
   [install it](https://docs.docker.com/desktop/install/windows-install/).

1. Open Docker Desktop and verify that it is running.

</div>

### Can't start Coder server: Address already in use

```shell
Encountered an error running "coder server", see "coder server --help" for more information
error: configure http(s): listen tcp 127.0.0.1:3000: bind: address already in use
```

Another process is already listening on port 3000. Identify and stop it,
then start the server again.

#### Linux

1. Stop the process:

   ```shell
   sudo systemctl stop coder
   ```

1. Start Coder:

   ```shell
   coder server
   ```

#### macOS

1. Identify the process using port 3000:

   ```shell
   lsof -i :3000
   ```

1. Stop the process using the PID from the previous command:

   ```shell
   kill <PID>
   ```

   If the process does not exit, force-kill it:

   ```shell
   kill -9 <PID>
   ```

1. Start Coder:

   ```shell
   coder server
   ```

#### Windows

1. Identify the process using port 3000 in PowerShell:

   ```powershell
   Get-NetTCPConnection -LocalPort 3000 | Select-Object OwningProcess
   ```

1. Stop the process using the PID from the previous command:

   ```powershell
   Stop-Process -Id <PID>
   ```

1. Start Coder:

   ```shell
   coder server
   ```
