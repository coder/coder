# Access your workspace

There are many ways to connect to your workspace, the options are only limited
by the template configuration.

Deployment operators can learn more about different types of workspace
connections and performance in our
[networking docs](../../admin/infrastructure/index.md).

You can see the primary methods of connecting to your workspace in the workspace
dashboard.

![Workspace View](../../images/user-guides/workspace-view-connection-annotated.png)

## Web Terminal

The Web Terminal is a browser-based terminal that provides instant access to
your workspace's shell environment. It uses [xterm.js](https://xtermjs.org/)
and WebSocket technology for a responsive terminal experience with features
like persistent sessions, Unicode support, and clickable URLs.

![Terminal Access](../../images/user-guides/terminal-access.png)

Read the complete [Web Terminal documentation](./web-terminal.md) for
customization options, keyboard shortcuts, and troubleshooting guides.

## SSH

### Through with the CLI

Coder will use the optimal path for an SSH connection (determined by your
deployment's [networking configuration](../../admin/infrastructure/index.md))
when using the CLI:

```console
coder ssh my-workspace
```

Or, you can configure plain SSH on your client below.

> [!Note]
> The `coder ssh` command does not have full parity with the standard
> SSH command. For users who need the full functionality of SSH, use the
> configuration method below.

### Running remote commands with quoting

Arguments after `--` are joined with spaces into a single command
string before being sent to the workspace agent (per
[RFC 4254 §6.5](https://www.rfc-editor.org/rfc/rfc4254#section-6.5),
which defines the SSH exec request as a single string). Your local
shell consumes outer quotes before the CLI ever sees the arguments,
so inner quoting is not preserved on the wire.

This means a common pattern from interactive shells does not behave
as you might expect:

```console
# Surprising: prints an empty line, NOT "ready"
coder ssh my-workspace -- bash -c 'echo ready'
```

The local shell strips the single-quotes, the CLI receives the argv
`[bash, -c, echo, ready]`, and the remote agent runs
`bash -c echo ready`; `echo` gets no arguments and `ready` is bound to
`$0`. The same caveat applies to plain `ssh user@host -- bash -c '...'`;
this is SSH protocol semantics, not a `coder ssh` limitation.

For commands that need preserved quoting, use one of these patterns:

**Heredoc via stdin** (recommended for multi-line scripts):

```console
coder ssh my-workspace -- bash <<'EOF'
echo ready
EOF
```

**A single-argument script payload**:

```console
coder ssh my-workspace -- /path/to/script.sh
```

**Exit-code-only probe** (when you only need to know if the command succeeded):

```console
coder ssh my-workspace -- true && echo "agent is reachable"
```

### Configure SSH

Coder generates [SSH key pairs](../../admin/security/secrets.md#ssh-keys) for
each user to simplify the setup process.

1. Use your terminal to authenticate the CLI with Coder web UI and your workspaces:

   ```bash
   coder login <accessURL>
   ```

1. Access Coder via SSH:

   ```shell
   coder config-ssh
   ```

1. Run `coder config-ssh --dry-run` if you'd like to see the changes that will be
   before you proceed:

   ```shell
   coder config-ssh --dry-run
   ```

1. Confirm that you want to continue by typing **yes** and pressing enter. If
successful, you'll see the following message:

   ```console
   You should now be able to ssh into your workspace.
   For example, try running:

   $ ssh coder.<workspaceName>
   ```

Your workspace is now accessible via `ssh coder.<workspace_name>`
(for example, `ssh coder.myEnv` if your workspace is named `myEnv`).

> [!TIP]
> If you use a third-party SSH client that discovers hosts by parsing
> `~/.ssh/config` (such as the VS Code Remote-SSH sidebar or scripts that
> enumerate known hosts), run `coder config-ssh --no-wildcard` instead. This
> generates an individual `Host` entry per workspace rather than a single
> wildcard block, making your workspaces visible to those tools.

## Visual Studio Code

You can develop in your Coder workspace remotely with
[VS Code](https://code.visualstudio.com/download).
We support connecting with the desktop client and VS Code in the browser with [code-server](#code-server).

![Demo](https://github.com/coder/vscode-coder/raw/main/demo.gif?raw=true)

Read more details on [using VS Code in your workspace](./vscode.md).

## Cursor

[Cursor](https://cursor.sh/) is an IDE built on VS Code with enhanced AI capabilities.
Cursor connects using the Coder extension.

Read more about [using Cursor with your workspace](./cursor.md).

## Windsurf

[Windsurf](./windsurf.md) is Codeium's code editor designed for AI-assisted development.
Windsurf connects using the Coder extension.

## Antigravity

[Antigravity](https://antigravity.google/) is Google's desktop IDE.
Antigravity connects using the Coder extension.

Read more about [using Antigravity with your workspace](./antigravity.md).

## JetBrains IDEs

We support JetBrains IDEs using
[Gateway](https://www.jetbrains.com/remote-development/gateway/). The following
IDEs are supported for remote development:

- IntelliJ IDEA
- CLion
- GoLand
- PyCharm
- Rider
- RubyMine
- WebStorm
- [JetBrains Fleet](./jetbrains/fleet.md)

Read our [docs on JetBrains](./jetbrains/index.md) for more information
on connecting your JetBrains IDEs.

## code-server

[code-server](https://github.com/coder/code-server) is our supported method of
running VS Code in the web browser.
Learn more about [what makes code-server different from VS Code web](./code-server.md) or visit the
[documentation for code-server](https://coder.com/docs/code-server).

![code-server in a workspace](../../images/code-server-ide.png)

## Other Web IDEs

We support a variety of other browser IDEs and tools to interact with your
workspace. Each of these can be configured by your template admin using our
[Web IDE guides](../../admin/templates/extending-templates/web-ides.md).

Supported IDEs:

- VS Code Web
- JupyterLab
- RStudio
- Airflow
- File Browser

Our [Module Registry](https://registry.coder.com/modules) also hosts a variety
of tools for extending the capability of your workspace. If you have a request
for a new IDE or tool, please file an issue in our
[Modules repo](https://github.com/coder/registry/issues).

## Coder Desktop

[Coder Desktop](../desktop/index.md) is a native application that provides seamless access to your workspaces via a VPN tunnel. With Coder Desktop, you get:

- **Automatic port forwarding**: All workspace ports are available at `workspace-name.coder:PORT` with no manual setup
- **SSH access**: Connect with `ssh workspace-name.coder` using any SSH client
- **File sync**: Bidirectional file synchronization between local and remote directories

Coder Desktop is the recommended way to access workspace services for developers who want a seamless, native experience.

## Ports and Port forwarding

You can manage listening ports on your workspace page through the listening
ports window in the dashboard. These ports are often used to run internal
services or preview environments.

> [!TIP]
> For automatic access to all ports without manual configuration, use [Coder Desktop](../desktop/index.md).

You can also [share ports](./port-forwarding.md#sharing-ports) with other users,
or [port-forward](./port-forwarding.md#the-coder-port-forward-command) through
the CLI with `coder port forward`. Read more in the
[docs on workspace ports](./port-forwarding.md).

![Open Ports window](../../images/networking/listeningports.png)

## Remote Desktops

Coder also supports connecting with an RDP solution, see our
[RDP guide](./remote-desktops.md) for details.
