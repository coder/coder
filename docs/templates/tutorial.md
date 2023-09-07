# Your first template

A common way to create a template is to begin with a starter template
then modify it for your needs. Coder makes this easy with starter
templates for popular development targets like Docker, Kubernetes,
Azure, and so on. Once your template is up and running, you can edit
it in the Coder web page. Coder even handles versioning for you so you
can publish official updates or revert to previous revisions.

In this tutorial, you'll create your first template from the Docker
starter template.


## Before you start

You'll need a computer or cloud computing instance with both
[Docker](https://docs.docker.com/get-docker/) and
[Coder](../install/index.md) installed on it.

## 1. Log in to Coder

In your web browser, go to your Coder instance to log in.

## 2. Choose a starter template

Select **Templates** > **Starter Templates**.

![Starter Templates button](../images/templates/starter-templates.png)

In **Filter**, select **Docker** then select **Develop in Docker**.

![Choosing a starter template](../images/templates/develop-in-docker-template.png)

Select **Use template**.

![Using a starter template](../images/templates/use-template.png)

## 3. Create your template

In **Create template**, fill in **Name** and **Display name**,then
scroll down and select **Create template**.

![Creating a template](../images/templates/create-template.png)

## 4. Create a workspace from your template

When the template is ready, select **Create Workspace**.

![Create workspace](../images/templates/create-workspace.png)

In **New workspace**, fill in **Name** then scroll down to select
**Create Workspace**.

![New workspace](../images/templates/new-workspace.png)

Coder starts your new workspace from your template.

## 5. Use your workspace

After a few seconds, you can use your workspace.

![Workspace is ready](../images/templates/workspace-ready.png)


## 6. Try out your new workspace

This starter template lets you connect to your workspace in a few ways:

- VS Code Desktop: Loads your workspace into [VS Code
  Desktop](https://code.visualstudio.com/Download) installed on your
  local computer.
- code-server: Opens [browser-based VS Code](../ides/web-ides.md)
  with your workspace.
- Terminal: Opens a browser-based terminal with a shell in the
  workspace's Docker instance.
- SSH: Use SSH to log in to the workspace from your local machine. If
  you haven't already, you'll have to install coder on your local
  machine to configure your SSH client.

You can edit the template to let developers connect to a workspace in
[a few more ways](../ides.md].

## Next steps
- [Anatomy of a template](./anatomy.md)
- [Setting up templates](./best-practices.md)
