# Visual Studio Code

You can develop in your Coder workspace remotely with
[VS Code](https://code.visualstudio.com/download).
We support connecting with the desktop client and VS Code in the browser with
[code-server](https://github.com/coder/code-server).
Learn more about how VS Code Web and code-server compare in the
[code-server doc](./code-server.md).

## VS Code Desktop

VS Code desktop is a default app for workspaces.

Click `VS Code Desktop` in the dashboard to one-click enter a workspace. This
automatically installs the [Coder Remote](https://github.com/coder/vscode-coder)
extension, authenticates with Coder, and connects to the workspace.

![Demo](https://github.com/coder/vscode-coder/raw/main/demo.gif?raw=true)

> [!NOTE]
> The `VS Code Desktop` button can be hidden by enabling
> [Browser-only connections](../../admin/networking/index.md#browser-only-connections).

### Manual Installation

You can install our extension manually in VS Code using the command palette.
Launch VS Code Quick Open (Ctrl+P), paste the following command, and press
enter.

```text
ext install coder.coder-remote
```

Alternatively, manually install the VSIX from the
[latest release](https://github.com/coder/vscode-coder/releases/latest).

## VS Code extensions

There are multiple ways to add extensions to VS Code Desktop:

1. Using the
   [public extensions marketplaces](#using-the-public-extensions-marketplaces)
   with Code Web (code-server)
1. Adding [extensions to custom images](#adding-extensions-to-custom-images)
1. Installing extensions
   [using its `vsix` file at the command line](#installing-extensions-using-its-vsix-file-at-the-command-line)
1. Installing extensions
   [from a marketplace using the command line](#installing-from-a-marketplace-at-the-command-line)

### Using the public extensions marketplaces

You can manually add an extension while you're working in the Code Web IDE. The
extensions can be from Coder's public marketplace, Eclipse Open VSX's public
marketplace, or the Eclipse Open VSX _local_ marketplace.

![Code Web Extensions](../../images/ides/code-web-extensions.png)

> [!NOTE]
> Microsoft does not allow any unofficial VS Code IDE to connect to the
> extension marketplace.

### Adding extensions to custom images

You can add extensions to a custom image and install them either through Code
Web or using the workspace's terminal.

1. Download the extension(s) from the Microsoft public marketplace.

   ![Code Web Extensions](../../images/ides/copilot.png)

1. Add the `vsix` extension files to the same folder as your Dockerfile.

   ```shell
   ~/images/base
    ➜  ls -l
    -rw-r--r-- 1 coder coder       0 Aug 1 19:23 Dockerfile
    -rw-r--r-- 1 coder coder 8925314 Aug 1 19:40 GitHub.copilot.vsix
   ```

1. In the Dockerfile, add instructions to make a folder and to copy the `vsix`
   files into the newly created folder.

   ```Dockerfile
   FROM codercom/enterprise-base:ubuntu

   # Run below commands as root user
   USER root

   # Download and install VS Code extensions into the container
   RUN mkdir -p /vsix
   ADD ./GitHub.copilot.vsix /vsix

   USER coder
   ```

1. Build the custom image, and push it to your image registry.

1. Pass in the image and below command into your template `startup_script` (be
   sure to update the filename below):

   **Startup Script**

   ```tf
   resource "coder_agent" "main" {
     ...
     startup_script = "code-server --install-extension /vsix/GitHub.copilot.vsix"
   }
   ```

   **Image Definition**

   ```tf
   resource "kubernetes_deployment" "main" {
     spec {
       template {
         spec {
           container {
             name   = "dev"
             image  = "registry.internal/image-name:tag"
           }
         }
       }
     }
   }
   ```

1. Create a workspace using the template.

You will now have access to the extension in your workspace.

### Installing extensions using its `vsix` file at the command line

Using the workspace's terminal or the terminal available inside `code-server`,
you can install an extension whose files you've downloaded from a marketplace:

```console
/path/to/code-server --install-extension /vsix/GitHub.copilot.vsix
```

### Installing from a marketplace at the command line

Using the workspace's terminal or the terminal available inside Code Web (code
server), run the following to install an extension (be sure to update the
snippets with the name of the extension you want to install):

```console
SERVICE_URL=https://extensions.coder.com/api ITEM_URL=https://extensions.coder.com/item /path/to/code-server --install-extension GitHub.copilot
```

Alternatively, you can install an extension from Open VSX's public marketplace:

```console
SERVICE_URL=https://open-vsx.org/vscode/gallery ITEM_URL=https://open-vsx.org/vscode/item /path/to/code-server --install-extension GitHub.copilot
```

### Using VS Code Desktop

For your local VS Code to pickup extension files in your Coder workspace,
include this command in your `startup_script`, or run in manually in your
workspace terminal:

```console
code --extensions-dir ~/.vscode-server/extensions --install-extension "$extension"
```

## Offline environments

### VS Code Remote SSH offline behavior

The Coder Remote extension uses VS Code's built-in Remote SSH extension
(`ms-vscode-remote.remote-ssh`) to connect to workspaces. This extension
requires specific network connectivity to function properly.

#### Network requirements

When connecting to a workspace, VS Code Remote SSH downloads the `vscode-server`
component to the remote workspace. This requires the **client machine** (your
desktop running VS Code) to have outbound HTTPS (port 443) connectivity to:

- `update.code.visualstudio.com`
- `vscode.blob.core.windows.net`
- `*.vo.msecnd.net`

> [!NOTE]
> The workspace itself does not need internet access. Only the VS Code client
> needs connectivity to download the server component, which is then transferred
> to the workspace via SSH.

#### Fully offline scenarios

**VS Code Remote SSH does not support scenarios where both the client and
workspace are fully offline** with no internet access. The extension requires
the client to download `vscode-server` during the initial connection.

If your client machine cannot access the required Microsoft domains, consider
these alternatives:

- **[code-server](./code-server.md)**: A browser-based VS Code that runs
  entirely within the workspace. See
  [air-gapped deployments](../../install/airgap.md) for offline setup.
- **[JetBrains IDEs](../../admin/templates/extending-templates/jetbrains-airgapped.md)**:
  Documented offline deployment steps for Gateway.

See [microsoft/vscode-remote-release#1242](https://github.com/microsoft/vscode-remote-release/issues/1242)
for more information about offline support limitations.

### How vscode-server is installed

VS Code Remote SSH installs `vscode-server` on the workspace through one of
these methods:

1. **Download via client** (default): The VS Code client downloads the server
   binary from Microsoft's CDN and transfers it to the workspace via `scp`.
   This is the standard method used by the Coder Remote extension.

2. **Direct download** (fallback): If the workspace has internet access and the
   client cannot transfer the file, the workspace may attempt to download
   `vscode-server` directly. This requires the workspace to access the same
   Microsoft domains listed above.

The server binary is version-specific and tied to a commit hash that matches
your VS Code client version. The binary is extracted to
`~/.vscode-server/bin/<COMMIT_ID>/` in the workspace.

### Manual vscode-server installation

In restricted environments where automated installation fails, you can manually
download and install `vscode-server`. This is **not officially supported by
Microsoft** and may break with VS Code updates.

> [!WARNING]
> Manual installation creates maintenance overhead. You must re-download and
> install `vscode-server` after every VS Code client update, as version
> mismatches will prevent connections.

#### Finding the commit ID

To find the commit ID for your VS Code version:

1. In VS Code, go to **Help** → **About**
2. Look for the "Commit" field (example:
   `92da9481c0904c6adfe372c12da3b7748d74bdcb`)

Alternatively, check the output in the **Remote - SSH** extension log when
attempting to connect to a workspace.

#### Download and installation steps

On a machine with internet access:

1. Download the server binary for your commit ID and platform:

   ```sh
   # For Linux x64 (most common)
   COMMIT_ID="92da9481c0904c6adfe372c12da3b7748d74bdcb"
   curl -Lo vscode-server.tar.gz \
     "https://update.code.visualstudio.com/commit:${COMMIT_ID}/server-linux-x64/stable"

   # For Alpine Linux (musl)
   curl -Lo vscode-server.tar.gz \
     "https://update.code.visualstudio.com/commit:${COMMIT_ID}/server-linux-alpine-x64/stable"

   # For ARM64
   curl -Lo vscode-server.tar.gz \
     "https://update.code.visualstudio.com/commit:${COMMIT_ID}/server-linux-arm64/stable"
   ```

2. Transfer the tarball to your workspace
3. On the workspace, extract and install:

   ```sh
   COMMIT_ID="92da9481c0904c6adfe372c12da3b7748d74bdcb"

   # Create the directory structure
   mkdir -p ~/.vscode-server/bin/${COMMIT_ID}

   # Extract the server
   tar -xzf vscode-server.tar.gz -C ~/.vscode-server/bin/${COMMIT_ID} --strip-components=1

   # Create marker file to indicate successful installation
   touch ~/.vscode-server/bin/${COMMIT_ID}/0
   ```

4. Attempt to connect from VS Code Desktop

See community resources for automated scripts:

- [Manual download script](https://gist.github.com/cvcore/8e187163f41a77f5271c26a870e52778)
- [Offline installation guide](https://gist.github.com/mansicer/9dd6a33beaf6852e841286c2511e25d5)

#### Known issues with manual installation

**Version mismatches**: When your VS Code client updates, the commit ID changes
and you must reinstall `vscode-server` with the new commit ID. Connections will
fail until versions match.

**Platform detection**: Ensure you download the correct platform binary
(Linux/Alpine/ARM) that matches your workspace architecture.

**Extension updates**: VS Code regularly releases updates that change server
requirements. Manual installations may break after VS Code auto-updates.

**Unsupported configuration**: Microsoft does not officially support manual
installation. Recent VS Code versions have made changes to the deployment
process that may affect manual installations.

#### Alternative: Mirror service

For organizations with many offline users, consider setting up an internal
mirror that mimics Microsoft's update service:

- Mirror the endpoints used by VS Code:
  - `https://update.code.visualstudio.com/commit:<COMMIT_ID>/server-<platform>/<channel>`
- Use network policies to redirect VS Code traffic to your mirror
- See [this example implementation](https://gist.github.com/b01/0a16b6645ab7921b0910603dfb85e4fb)
  for inspiration

This approach centralizes version management but requires infrastructure and
ongoing maintenance to stay current with VS Code releases.
