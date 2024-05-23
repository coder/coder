# Visual Studio Code

You can develop in your Coder workspace remotely with [VSCode desktop](https://code.visualstudio.com/download).   

Click `VS Code Desktop` in the dashboard to one-click enter a workspace. This
automatically installs the [Coder Remote](https://github.com/coder/vscode-coder)
extension, authenticates with Coder, and connects to the workspace.

![Demo](https://github.com/coder/vscode-coder/raw/main/demo.gif?raw=true)

You can set the default directory in which VS Code opens via the `dir` argument
on the `coder_agent` resource in your workspace template. See the
[Terraform documentation for more details](https://registry.terraform.io/providers/coder/coder/latest/docs/resources/agent#dir).

> The `VS Code Desktop` button can be hidden by enabling
> [Browser-only connections](./networking/index.md#Browser-only).
