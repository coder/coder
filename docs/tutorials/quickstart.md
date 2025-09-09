# Get Started

** Keywords: ** install, setup, get started, quickstart, templates, workspaces, tasks, users

Follow the steps in this guide to get your first Coder development environment running in under 10 minutes. This guide covers the essential concepts and walks you through creating your first workspace and running VS Code from it. You can also get Claude Code up and running in the background!

## What You'll Build

In this quickstart, you'll:
- ✅ Install Coder server
- ✅ Create a **template** (blueprint for dev environments)
- ✅ Launch a **workspace** (your actual dev environment)
- ✅ Connect from your favorite IDE
- ✅ Optionally setup a **task** running Claude Code 

## Understanding Coder: 30-Second Overview

Before diving in, here are the four concepts that power Coder explained through a cooking analogy:

| Component | What It Is | Real-World Analogy |
|-----------|------------|-------------------|
| **You** | The engineer/developer/builder working | The head chef cooking the meal |
| **Templates** | A Terraform blueprint that defines your dev environment (OS, tools, resources) | Recipe for a meal |
| **Workspaces** | The actual running environment created from the template | The cooked meal |
| **Tasks** | AI-powered coding agents that run inside a workspace | Your sous chef helping you cook the meal |
| **Users** | A developer who launches the workspace from a template and does their work inside it | The people eating the meal |

**First time here?** Coder separates how an environment is defined (Admin’s job) from where you do your day-to-day coding (Developer’s job). As a developer, you’ll use templates to launch workspaces, and as an admin, you’ll create and manage those templates for others.

## Prerequisites

- A machine with 2+ CPU cores and 4GB+ RAM
- 10 minutes of your time

## Step 1: Install Docker and Setup Permissions

<div class="tabs">

## Linux/macOS

1. Install Docker:

   ```bash
   curl -sSL https://get.docker.com | sh
   ```

   For more details, visit:

   - [Linux instructions](https://docs.docker.com/desktop/install/linux-install/)
   - [Mac instructions](https://docs.docker.com/desktop/install/mac-install/)

1. Assign your user to the Docker group:

   ```shell
   sudo usermod -aG docker $USER
   ```

1. Run `newgrp` to activate the groups changes:

   ```shell
   newgrp docker
   ```

   You might need to log out and back in or restart the machine for changes to
   take effect.

## Windows

If you plan to use the built-in PostgreSQL database, ensure that the
[Visual C++ Runtime](https://learn.microsoft.com/en-US/cpp/windows/latest-supported-vc-redist#latest-microsoft-visual-c-redistributable-version)
is installed.

1. [Install Docker](https://docs.docker.com/desktop/install/windows-install/).

</div>

## Step 2: Install & Start Coder

The `coder` CLI is all you need to install. It let's you run both the Coder server as well as the client. 

<div class="tabs">

## Linux/macOS

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

## Windows

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


## Configure Coder with a new Workspace

1. Coder will attempt to open the setup page in your browser. If it doesn't open
   automatically, go to <http://localhost:3000>.

   - If you get a browser warning similar to `Secure Site Not Available`, you
     can ignore the warning and continue to the setup page.

   If your Coder server is on a network or cloud device, or you are having
   trouble viewing the page, locate the web UI URL in Coder logs in your
   terminal. It looks like `https://<CUSTOM-STRING>.<TUNNEL>.try.coder.app`.
   It's one of the first lines of output, so you might have to scroll up to find
   it.

1. On the **Welcome to Coder** page, to use your GitHub account to log in,
   select **Continue with GitHub**.
   You can also enter an email and password to create a new admin account on
   the Coder deployment:

   ![Welcome to Coder - Create admin user](../images/screenshots/welcome-create-admin-user.png)_Welcome
   to Coder - Create admin user_

1. On the **Workspaces** page, select **Go to templates** to create a new
   template.

1. For this guide, use a Docker container. Locate **Docker Containers** and
   select **Use template**.

1. Give the template a **Name** that you'll recognize both in the Coder UI and
   in command-line calls.

   The rest of the template details are optional, but will be helpful when you
   have more templates.

   ![Create template](../images/screenshots/create-template.png)_Create
   template_

1. Select **Save**.

1. After the template is ready, select **Create Workspace**.

1. Give the workspace a name and select **Create Workspace**.

1. Coder starts your new workspace:

   ![getting-started-workspace is running](../images/screenshots/workspace-running-with-topbar.png)_Workspace
   is running_

1. Select **VS Code Desktop** to install the Coder extension and connect to your
   Coder workspace.

## Work on some code

After VS Code loads the remote environment, you can select **Open Folder** to
explore directories in the Docker container or work on something new.

To clone an existing repository:

1. Select **Clone Repository** and enter the repository URL.

   For example, to clone the Coder repo, enter
   `https://github.com/coder/coder.git`.

   Learn more about how to find the repository URL in the
   [GitHub documentation](https://docs.github.com/en/repositories/creating-and-managing-repositories/cloning-a-repository).

1. Choose the folder to which VS Code should clone the repo. It will be in its
   own directory within this folder.

   Note that you cannot create a new parent directory in this step.

1. After VS Code completes the clone, select **Open** to open the directory.

1. You are now using VS Code in your Coder environment!

## What's next?

Now that you have your own workspace, use the same template to set one up for a
teammate.

Go to **Templates** and select **Create Workspace** and continue from Step 7 in
[Configure Coder with a new workspace](#configure-coder-with-a-new-workspace).

After that, you can try to:

- [Customize templates](../admin/templates/extending-templates/index.md)
- [Enable Prometheus metrics](../admin/integrations/prometheus.md)
- [Deploy to Google Cloud Platform (GCP)](../install/cloud/compute-engine.md)

## Troubleshooting

### Cannot connect to the Docker daemon

> Error: Error pinging Docker server: Cannot connect to the Docker daemon at
> unix:///var/run/docker.sock. Is the docker daemon running?

1. Install Docker for your system:

   ```shell
   curl -sSL https://get.docker.com | sh
   ```

1. Set up the Docker daemon in rootless mode for your user to run Docker as a
   non-privileged user:

   ```shell
   dockerd-rootless-setuptool.sh install
   ```

   Depending on your system's dependencies, you might need to run other commands
   before you retry this step. Read the output of this command for further
   instructions.

1. Assign your user to the Docker group:

   ```shell
   sudo usermod -aG docker $USER
   ```

1. Confirm that the user has been added:

   ```console
   $ groups
   docker sudo users
   ```

   - Ubuntu users might not see the group membership update. In that case, run
     the following command or reboot the machine:

     ```shell
     newgrp docker
     ```

### Can't start Coder server: Address already in use

```shell
Encountered an error running "coder server", see "coder server --help" for more information
error: configure http(s): listen tcp 127.0.0.1:3000: bind: address already in use
```

1. Stop the process:

   ```shell
   sudo systemctl stop coder
   ```

1. Start Coder:

   ```shell
   coder server
   ```
