# Visual Studio Code in air-gapped environments

Many Coder administrators deploy Coder in [air-gapped environments](../../install/offline.md) where the server or workspaces do not have network access to external websites such as `registry.terraform.com` or `code.visualstudio.com`.

## Allow network access to the VS Code Extension Marketplace

If some network access is allowed, you can allow users access to `code.visualstudio.com`.

When they connect to a workspace with VS Code Remote SSH, their desktop will download the server and scp it into the workspace.

## Portable mode

VS Code [Portable mode](https://code.visualstudio.com/docs/editor/portable) is a self-contained, or "portable," installation of VS Code that uses local files for both installation and application data such as extensions.
