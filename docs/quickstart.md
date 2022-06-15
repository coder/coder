# Quickstart

This guide will walk you through creating your first template and workspace.

## Prerequisites

Please [install Coder](./install.md) before proceeding with the steps outlined in this article.

## Creating your first template and workspace

In a new terminal window, run the following to copy a sample template:

```bash
coder templates init
```

Follow the CLI instructions to select an example that you can modify for your
specific usage (e.g., a template to **Develop code-server in Docker**):

1. Navigate into your new templates folder and create your first template using
   the provided command (e.g., `cd ./docker-code-server && coder templates create`)

1. Answer the CLI prompts; when done, confirm that you want to create your template.

Create a workspace using your template:

```bash
coder create --template="yourTemplate" <workspaceName>
```

Connect to your workspace via SSH:

```bash
coder ssh <workspaceName>
```

You can also access your workspace using the **access URL** you provided when
deploying Coder (if you're using a temporary deployment and you opted to use
Coder's tunnel, use the access URL you were provided). Log in with the admin
credentials provided to you by Coder.

![Coder Web UI with code-server](images/code-server.png)

## Modifying templates

You can edit the Terraform template as follows:

```sh
coder templates init
cd gcp-linux # modify this line as needed to access the template
vim main.tf
coder templates update gcp-linux # updates the template
```
