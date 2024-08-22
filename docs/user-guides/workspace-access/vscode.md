# Visual Studio Code

You can develop in your Coder workspace remotely with
[VSCode](https://code.visualstudio.com/download). We support connecting with the
desktop client and VSCode in the browser with [code-server](#code-server).

## VSCode Desktop

VSCode desktop is a default app for workspaces.

Click `VS Code Desktop` in the dashboard to one-click enter a workspace. This
automatically installs the [Coder Remote](https://github.com/coder/vscode-coder)
extension, authenticates with Coder, and connects to the workspace.

![Demo](https://github.com/coder/vscode-coder/raw/main/demo.gif?raw=true)

> The `VS Code Desktop` button can be hidden by enabling
> [Browser-only connections](./networking/index.md#Browser-only).

### Manual Installation

You can install our extension manually in VSCode using the command palette.
Launch VS Code Quick Open (Ctrl+P), paste the following command, and press
enter.

```text
ext install coder.coder-remote
```

Alternatively, manually install the VSIX from the
[latest release](https://github.com/coder/vscode-coder/releases/latest).

> **Note**: To configure the extensions and default directory of VS Code, A
> template administrator can follow the guide on
> [Extending Templates](../../admin/templates/extending-templates/ides/vscode-desktop.md).
