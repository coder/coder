# Windsurf

[Windsurf](https://codeium.com/windsurf) is Codeium's code editor designed for AI-assisted
development.

Follow this guide to use Windsurf to access your Coder workspaces.

If your team uses Windsurf regularly, ask your Coder administrator to add Windsurf as a workspace application in your template.

## Install Windsurf and Coder CLI

Windsurf can connect to your Coder workspaces via SSH:

1. [Install Windsurf](https://docs.codeium.com/windsurf/getting-started) on your local machine.

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

1. Open Windsurf and select **Get started**.

   Import your settings from another IDE, or select **Start fresh**.

1. Complete the setup flow and log in or [create a Codeium account](https://codeium.com/windsurf/signup)
   if you don't have one already.

## Install the Coder extension

Use the Coder extension to connect to your workspaces through SSH.

1. Download the [latest vscode-coder extension](https://github.com/coder/vscode-coder/releases/latest) `.vsix` file.

1. Drag the `.vsix` file into the extensions pane of Windsurf.

   Alternatively:

   1. Open the Command Palette
   (<kdb>Ctrl</kdb>+<kdb>Shift</kdb>+<kdb>P</kdb> or <kdb>Cmd</kdb>+<kdb>Shift</kdb>+<kdb>P</kdb>)
   and search for `vsix`.

   1. Select **Extensions: Install from VSIX** and select the vscode-coder extension you downloaded.

## Open a workspace in Windsurf

1. From the Windsurf Command Palette
(<kdb>Ctrl</kdb>+<kdb>Shift</kdb>+<kdb>P</kdb> or <kdb>Cmd</kdb>+<kdb>Shift</kdb>+<kdb>P</kdb>),
enter `coder` and select **Coder: Login**.

1. Follow the prompts to login and copy your session token.

   Paste the session token in the **Coder API Key** dialogue in Windsurf.

1. Windsurf prompts you to open a workspace, or you can use the Command Palette to run **Coder: Open Workspace**.

> [!NOTE]
> If you have any suggestions or experience any issues, please
> [create a GitHub issue](https://github.com/coder/coder/issues/new?title=docs%3A+windsurf+request+title+here&labels=["customer-reported","docs"]&body=please+enter+your+request+here) or share in
> [our Discord channel](https://discord.gg/coder).
