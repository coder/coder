# Tour Coder and with your first deployment

This guide walks through the core product concepts by setting up a Coder
deployment, creating your first template, and connecting to a workspace. This is
made readily available by leveraging our
[open source repository](https://github.com/coder/coder).

We'll use [Docker](https://docs.docker.com/engine) to manage the compute for a
slim deployment.

# Set up your Coder Deployment

### 1. Install Docker

First, install [Docker](https://docs.docker.com/engine/install) locally.

> If you already have the Coder binary installed, restart it after installing
> Docker.

### 2. Install Coder daemon

A single binary contains the CLI (`coder`) and is used for both the Coder server
and the client.

<div class="tabs">

## Linux/macOS

Our install script is the fastest way to install Coder on Linux/macOS:

```sh
curl -L https://coder.com/install.sh | sh
```

## Windows

You can use the
[`winget`](https://learn.microsoft.com/en-us/windows/package-manager/winget/#use-winget)
package manager to install Coder:

```powershell
winget install Coder.Coder
```

> **Important:** If you plan to use the built-in PostgreSQL database, you will
> need to ensure that the
> [Visual C++ Runtime](https://learn.microsoft.com/en-US/cpp/windows/latest-supported-vc-redist#latest-microsoft-visual-c-redistributable-version)
> is installed.

</div>

> See our [install docs](../install/README.md) for other methods of installing
> and deploying Coder.

### 3. Start the server

To start or restart the Coder deployment, use the following command:

```shell
coder server
```

The output will provide you with a URL to access your deployment, where you'll
create your first administrator account.

![Coder login screen](../images/start/setup-page.png)

Once you've signed in, you'll be brought to an empty workspaces page, which
we'll soon populate with your first development environments.

> To learn more about interacting with the control plane through the terminal,
> see our `coder server` [CLI docs](../reference/cli/server.md).

## Create your first template

A common way to create a template is to begin with a starter template then
modify it for your needs. Coder makes this easy with starter templates for
popular development targets like Docker, Kubernetes, Azure, and so on. Once your
template is up and running, you can edit it in the Coder dashboard. Coder even
handles versioning for you so you can publish official updates or revert to
previous versions.

> Read more about templates in our
> [tutorials section](../admin/templates/README.md).

In this tutorial, you'll create your first template from the Docker starter
template.

### 1. Choose a starter template

Select **Templates** to see the **Starter Templates**. Use the **Docker
Containers** template by pressing **Use Template**.

![Starter Templates UI](../images/start/starter-templates-annotated.png)

> You can also a find a comprehensive list of starter templates in **Templates**
> -> **Create Template** -> **Starter Templates**.

### 2. Create your template

In **Create template**, fill in **Name** and **Display name**, then select
**Create template**.

![Creating a template](../images/start/create-template.png)

### Other template resources

- [Templates overview](../admin/templates/README.md)
- [Create templates through the CLI](../cli/templates_create.md)
- See our
  [Community Templates](https://github.com/coder/coder/blob/main/examples/templates/community-templates.md)
  in Github
- Create a template from a [Dev Container](../templates/dev-containers.md)

# Create a workspace

Coder workspaces are the primary interface for end-users. Each workspace is
defined by a template, but can be modified with parameters and settings. The
easiest way to create a workspace is through the template GUI.

> Read more about templates in our
> [administration docs](../admin/templates/README.md).

### 1. Create a workspace from your template

From the template page, select **Create Workspace**.

![Template Preview](../images/start/template-preview.png)

In **New workspace**, fill in **Name** then scroll down to select **Create
Workspace**.

![Create Workspace](../images/start/create-workspace.png)

Coder starts your new workspace from your template.

After a few seconds, your workspace is ready to use.

![Workspace is ready](../images/start/workspace-ready.png)

### 2. Try out your new workspace

This starter template lets you connect to your workspace in a few ways:

- VS Code Desktop: Loads your workspace into
  [VS Code Desktop](https://code.visualstudio.com/Download) installed on your
  local computer.
- code-server: Opens [browser-based VS Code](../ides/web-ides.md) with your
  workspace.
- Terminal: Opens a browser-based terminal with a shell in the workspace's
  Docker instance.
- SSH: Use SSH to log in to the workspace from your local machine. If you
  haven't already, you'll have to install Coder on your local machine to
  configure your SSH client.

> **Tip**: You can edit the template to let developers connect to a workspace in
> [a few more ways](../ides.md).

When you're done, you can stop the workspace.

### Modify your template

Now you can modify your template to suit your team's needs.

Let's replace the `golang` package in the Docker image with the `python3`
package. You can do this by editing the template's `Dockerfile` directly in your
web browser.

In the Coder dashboard, select **Templates** then your first template.

![Selecting the first template](../images/start/first-template.png)

In the this view, select **Source Code** to view and edit the template files.

![Edit template files](../images/start/template-source-code.png)

Click the **Edit** button at the top-right of the `build/Dockerfile` to open the
in-browser editor then replace `golang` with `python3`.

![Editing source code](../images/start/template-edit-source-code.png)

Select **Build template** and wait for Coder to prepare the template for
workspaces.

![Building a template](../images/start/build-template.png)

Select **Publish version**. In the **Publish new version** dialog, make sure
**Promote to default version** is checked then select **Publish**.

![Publish a template](../images/start/template-publish.png)

Now when developers create a new workspace from this template, they can use
Python 3 instead of Go.

For developers with workspaces that were created with a previous version of your
template, Coder will notify them that there's a new version of the template.

You can also handle [change management](./change-management.md) through your own
repo and continuous integration.

### Next Steps

- TODO
