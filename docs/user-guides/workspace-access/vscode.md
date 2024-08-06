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

You can set the default directory in which VS Code opens via the `dir` argument
on the `coder_agent` resource in your workspace template. See the
[Terraform documentation for more details](https://registry.terraform.io/providers/coder/coder/latest/docs/resources/agent#dir).

> The `VS Code Desktop` button can be hidden by enabling
> [Browser-only connections](./networking/index.md#Browser-only).

### Manual Installation

You can install our extension manually in VSCode using the command pallette.
Launch VS Code Quick Open (Ctrl+P), paste the following command, and press
enter.

```text
ext install coder.coder-remote
```

Alternatively, manually install the VSIX from the
[latest release](https://github.com/coder/vscode-coder/releases/latest).

## code-server

[code-server](https://github.com/coder/code-server) is our supported method of
running VS Code in the web browser. You can read more in our
[documentation for code-server](https://coder.com/docs/code-server/latest).

![code-server in a workspace](../../images/code-server-ide.png)
