# Cursor

[Cursor](https://cursor.sh/) is a modern IDE built on top of VS Code with enhanced AI capabilities.

## Connect to Coder via SSH

Cursor can connect to a Coder workspace using SSH:

1. [Install Cursor](https://cursor.sh/) on your local machine
1. Install the Coder CLI:

   <!-- copied from docs/install/cli.md - make changes there -->

   <div class="tabs">

   ### Linux/macOS

   Our install script is the fastest way to install Coder on Linux/macOS:

   ```sh
   curl -L https://coder.com/install.sh | sh
   ```

   Refer to [GitHub releases](https://github.com/coder/coder/releases) for
   alternate installation methods (e.g. standalone binaries, system packages).

   ### Windows

   Use [GitHub releases](https://github.com/coder/coder/releases) to download the
   Windows installer (`.msi`) or standalone binary (`.exe`).

   ![Windows setup wizard](../../images/install/windows-installer.png)

   Alternatively, you can use the
   [`winget`](https://learn.microsoft.com/en-us/windows/package-manager/winget/#use-winget)
   package manager to install Coder:

   ```powershell
   winget install Coder.Coder
   ```

   </div>

   Consult the [Coder CLI documentation](../../install/cli.md) for more options.

1. Log in to your Coder deployment and authenticate when prompted:

   ```shell
   coder login coder.example.com
   ```

1. Configure Coder SSH:

   ```shell
   coder config-ssh
   ```

1. List your available workspaces:

   ```shell
   coder list
   ```

1. Open Cursor

1. Download [Open Remote - SSH](https://open-vsx.org/extension/jeanp413/open-remote-ssh).

1. Download the [latest vscode-coder extension](https://github.com/coder/vscode-coder/releases/latest).

1. Open the Command Palette (<kdb>Ctrl</kdb>+<kdb>Shift</kdb>+<kdb>P</kdb> or <kdb>Cmd</kdb>+<kdb>Shift</kdb>+<kdb>P</kdb>) and search for `vsix`.

1. Select **Extensions: Install from VSIX** and select the extensions you downloaded.

1. Select **Connect via SSH** and enter the workspace name as `coder.workspace-name`.

1. After you connect, select **Open Folder** and you can start working on your files.

> [!NOTE]
> If you have any suggestions or experience any issues, please
> [create a GitHub issue](https://github.com/coder/coder/issues/new?title=docs%3A+cursor+request+title+here&labels=["customer-reported","docs"]&body=please+enter+your+request+here) or share in
> [our Discord channel](https://discord.gg/coder).
