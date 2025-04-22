# Coder Desktop (Beta)

Use Coder Desktop to work on your workspaces as though they're on your LAN, no
port-forwarding required.

> [!NOTE]
> Coder Desktop requires a Coder deployment running [v2.20.0](https://github.com/coder/coder/releases/tag/v2.20.0) or later.

## Install Coder Desktop

<div class="tabs">

You can install Coder Desktop on macOS or Windows.

### macOS

1. Use [Homebrew](https://brew.sh/) to install Coder Desktop:

   ```shell
   brew install --cask coder/coder/coder-desktop
   ```

   Alternatively, you can manually install Coder Desktop from the [releases page](https://github.com/coder/coder-desktop-macos/releases).

1. Open **Coder Desktop** from the Applications directory.

1. The application is treated as a system VPN. macOS will prompt you to confirm with:

   **"Coder Desktop" would like to use a new network extension**

   Select **Open System Settings**.

1. In the **Network Extensions** system settings, enable the Coder Desktop extension.

1. Continue to the [configuration section](#configure).

> Do not install more than one copy of Coder Desktop.
>
> To avoid system VPN configuration conflicts, only one copy of `Coder Desktop.app` should exist on your Mac, and it must remain in `/Applications`.

### Windows

1. Download the latest `CoderDesktop` installer executable (`.exe`) from the [coder-desktop-windows release page](https://github.com/coder/coder-desktop-windows/releases).

   Choose the architecture that fits your Windows system, `x64` or `arm64`.

1. Open the `.exe` file, acknowledge the license terms and conditions, and select **Install**.

1. If a suitable .NET runtime is not already installed, the installation might prompt you with the **.NET Windows Desktop Runtime** installation.

   In that installation window, select **Install**. Select **Close** when the runtime installation completes.

1. When the Coder Desktop installation completes, select **Close**.

1. Find and open **Coder Desktop** from your Start Menu.

1. Some systems require an additional Windows App Runtime SDK.

   Select **Yes** if you are prompted to install it.
   This will open your default browser where you can download and install the latest stable release of the Windows App Runtime SDK.

   Reopen Coder Desktop after you install the runtime.

1. Coder Desktop starts minimized in the Windows System Tray.

   You might need to select the **^** in your system tray to show more icons.

1. Continue to the [configuration section](#configure).

</div>

## Configure

Before you can use Coder Desktop, you will need to sign in.

1. Open the Desktop menu and select **Sign in**:

   <div class="tabs">

   ## macOS

   <Image height="325px" src="../../images/user-guides/desktop/coder-desktop-mac-pre-sign-in.png" alt="Coder Desktop menu before the user signs in" align="center" />

   ## Windows

   <Image height="325px" src="../../images/user-guides/desktop/coder-desktop-win-pre-sign-in.png" alt="Coder Desktop menu before the user signs in" align="center" />

   </div>

1. In the **Sign In** window, enter your Coder deployment's URL and select **Next**:

   ![Coder Desktop sign in](../../images/user-guides/desktop/coder-desktop-sign-in.png)

1. macOS: Select the link to your deployment's `/cli-auth` page to generate a [session token](../../admin/users/sessions-tokens.md).

   Windows: Select **Generate a token via the Web UI**.

1. In your web browser, you may be prompted to sign in to Coder with your credentials:

   <Image height="412px" src="../../images/templates/coder-login-web.png" alt="Sign in to your Coder deployment" align="center" />

1. Copy the session token to the clipboard:

   <Image height="350px" src="../../images/templates/coder-session-token.png" alt="Copy session token" align="center" />

1. Paste the token in the **Session Token** field of the **Sign In** screen, then select **Sign In**:

   ![Paste the session token in to sign in](../../images/user-guides/desktop/coder-desktop-session-token.png)

1. macOS: Allow the VPN configuration for Coder Desktop if you are prompted.

   <Image height="350px" src="../../images/user-guides/desktop/mac-allow-vpn.png" alt="Copy session token" align="center" />

1. Select the Coder icon in the menu bar (macOS) or system tray (Windows), and click the **Coder Connect** toggle to enable the connection.

   ![Coder Desktop on Windows - enable Coder Connect](../../images/user-guides/desktop/coder-desktop-win-enable-coder-connect.png)

   This may take a few moments, as Coder Desktop will download the necessary components from the Coder server if they have been updated.

1. macOS: You may be prompted to enter your password to allow Coder Connect to start.

1. Coder Connect is now running!

## Coder Connect

While active, Coder Connect will list the workspaces you own and will configure your system to connect to them over private IPv6 addresses and custom hostnames ending in `.coder`.

![Coder Desktop list of workspaces](../../images/user-guides/desktop/coder-desktop-workspaces.png)

To copy the `.coder` hostname of a workspace agent, you can click the copy icon beside it.

You can also connect to the SSH server in your workspace using any SSH client, such as OpenSSH or PuTTY:

   ```shell
   ssh your-workspace.coder
   ```

Any services listening on ports in your workspace will be available on the same hostname. For example, you can access a web server on port `8080` by visiting `http://your-workspace.coder:8080` in your browser.

> [!NOTE]
> Currently, the Coder IDE extensions for VSCode and JetBrains create their own tunnel and do not utilize the Coder Connect tunnel to connect to workspaces.

### Ping your workspace

<div class="tabs">

### macOS

Use `ping6` in your terminal to verify the connection to your workspace:

   ```shell
   ping6 -c 5 your-workspace.coder
   ```

### Windows

Use `ping` in a Command Prompt or PowerShell terminal to verify the connection to your workspace:

   ```shell
   ping -n 5 your-workspace.coder
   ```

</div>

## Sync a local directory with your workspace

Coder Desktop file sync provides bidirectional synchronization between a local directory and your workspace.
You can work offline, add screenshots to documentation, or use local development tools while keeping your files in sync with your workspace.

1. Create a new local directory.

   If you select an existing clone of your repository, Desktop will recognize it as conflicting files.

1. In the Coder Desktop app, select **File sync**.

   ![Coder Desktop File Sync screen](../../images/user-guides/desktop/coder-desktop-file-sync.png)

1. Select the **+** in the corner to select the local path, workspace, and remote path, then select **Add**:

   ![Coder Desktop File Sync add paths](../../images/user-guides/desktop/coder-desktop-file-sync-add.png)

1. File sync clones your workspace directory to your local directory, then watches for changes:

   ![Coder Desktop File Sync watching](../../images/user-guides/desktop/coder-desktop-file-sync-watching.png)

    For more information about the current status, hover your mouse over the status.

File sync excludes version control system directories like `.git/` from synchronization, so keep your Git-cloned repository wherever you run Git commands.
This means that if you use an IDE with a built-in terminal to edit files on your remote workspace, that should be the Git clone and your local directory should be for file syncs.

If you encounter a synchronization conflict, delete the conflicting file that contains changes you don't want to keep.

## Accessing web apps in a secure browser context

Some web applications require a [secure context](https://developer.mozilla.org/en-US/docs/Web/Security/Secure_Contexts) to function correctly.
A browser typically considers an origin secure if the connection is to `localhost`, or over `HTTPS`.

As Coder Connect uses its own hostnames and does not provide TLS to the browser, Google Chrome and Firefox will not allow any web APIs that require a secure context.

> [!NOTE]
> Despite the browser showing an insecure connection without `HTTPS`, the underlying tunnel is encrypted with WireGuard in the same fashion as other Coder workspace connections (e.g. `coder port-forward`).

If you require secure context web APIs, you will need to mark the workspace hostnames as secure in your browser settings.

We are planning some changes to Coder Desktop that will make accessing secure context web apps easier. Stay tuned for updates.

<div class="tabs">

### Chrome

1. Open Chrome and visit `chrome://flags/#unsafely-treat-insecure-origin-as-secure`.

1. Enter the full workspace hostname, including the `http` scheme and the port (e.g. `http://your-workspace.coder:8080`), into the **Insecure origins treated as secure** text field.

   If you need to enter multiple URLs, use a comma to separate them.

   ![Google Chrome insecure origin settings](../../images/user-guides/desktop/chrome-insecure-origin.png)

1. Ensure that the dropdown to the right of the text field is set to **Enabled**.

1. You will be prompted to relaunch Google Chrome at the bottom of the page. Select **Relaunch** to restart Google Chrome.

1. On relaunch and subsequent launches, Google Chrome will show a banner stating "You are using an unsupported command-line flag". This banner can be safely dismissed.

1. Web apps accessed on the configured hostnames and ports will now function correctly in a secure context.

### Firefox

1. Open Firefox and visit `about:config`.

1. Read the warning and select **Accept the Risk and Continue** to access the Firefox configuration page.

1. Enter `dom.securecontext.allowlist` into the search bar at the top.

1. Select **String** on the entry with the same name at the bottom of the list, then select the plus icon on the right.

1. In the text field, enter the full workspace hostname, without the `http` scheme and port: `your-workspace.coder`. Then select the tick icon.

   If you need to enter multiple URLs, use a comma to separate them.

   ![Firefox insecure origin settings](../../images/user-guides/desktop/firefox-insecure-origin.png)

1. Web apps accessed on the configured hostnames will now function correctly in a secure context without requiring a restart.

</div>

## Troubleshooting

### Mac: Issues updating Coder Desktop

> No workspaces!

And

> Internal Error: The VPN must be started with the app open during first-time setup.

Due to an issue with the way Coder Desktop works with the macOS [interprocess communication mechanism](https://developer.apple.com/documentation/xpc)(XPC) system network extension, core Desktop functionality can break when you upgrade the application.

<div class="tabs">

The resolution depends on which version of macOS you use:

### macOS <=14

1. Delete the application from `/Applications`.
1. Restart your device.

### macOS 15+

1. Open **System Settings**
1. Select **General**
1. Select **Login Items & Extensions**
1. Scroll down, and select the **ⓘ** for **Network Extensions**
1. Select the **...** next to Coder Desktop, then **Delete Extension**, and follow the prompts.
1. Re-open Coder Desktop and follow the prompts to reinstall the network extension.

</div>
