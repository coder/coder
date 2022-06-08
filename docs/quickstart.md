# Quickstart

This guide will walk you through creating your first template and workspace. If you haven't already installed `coder`, do that first [here](./install.md).

## Creating your first template and workspace

In a new terminal window, run the following to copy a sample template:

```bash
coder templates init
```

Follow the CLI instructions to modify and create the template specific for your
usage (e.g., a template to **Develop in Linux on Google Cloud**).

Create a workspace using your template:

```bash
coder create --template="yourTemplate" <workspaceName>
```

Connect to your workspace via SSH:

```bash
coder ssh <workspaceName>
```

## Modifying templates

If needed, you can edit the Terraform template using a sample template:

```sh
coder templates init
cd gcp-linux/
vim main.tf
coder templates update gcp-linux
```
