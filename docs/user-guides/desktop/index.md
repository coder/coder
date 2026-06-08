# Install Coder Desktop

Coder Desktop is a native macOS and Windows application that connects your local
machine to your remote workspaces over a secure tunnel. Once installed, you can
reach any workspace at `workspace-name.coder` from your terminal, browser, IDE,
or SSH client, with no CLI setup and no manual port forwarding.

> [!TIP]
> Coder Desktop provides **automatic port forwarding** to every service running
> in your workspace. Any port your application listens on is instantly
> accessible at `workspace-name.coder:PORT` with no manual setup required. For
> a comparison of all port forwarding methods, see
> [Workspace Ports](../workspace-access/port-forwarding.md).

## What is Coder Desktop?

Coder Desktop runs **Coder Connect**, a secure tunnel between your machine and
your Coder deployment. With Coder Connect enabled, your workspaces appear as
regular hosts on your local network, so any tool that speaks TCP works without
extra configuration.

- **Direct workspace hostnames**: Reach workspaces at `workspace-name.coder`.
  No SSH config, no `coder port-forward` commands.
- **All workspace ports, automatically**: Every port your workspace exposes is
  available at `workspace-name.coder:PORT`. No allowlists, no per-port setup.
- **Any local app, any protocol**: SSH clients, browsers, IDEs, database GUIs,
  and HTTP/gRPC clients connect like the workspace is on your LAN.
- **File sync**: Mirror a workspace directory to a local folder for offline
  editing and fast local tooling.
- **Split tunnel**: Only Coder traffic is routed through the tunnel. The rest
  of your internet stays untouched.

For a deeper walkthrough of Coder Connect and file sync, see
[Using Coder Connect and File Sync](./desktop-connect-sync.md).

## Install

Coder Desktop requires a Coder deployment running `v2.20.0` or
[later](https://github.com/coder/coder/releases/latest) and your deployment
URL. Installation requires administrator privileges on your local machine to
register the VPN extension.

### macOS

Install with Homebrew:

```shell
brew install --cask coder/coder/coder-desktop
```

Or download `Coder-Desktop.pkg` from the
[coder-desktop-macos releases page](https://github.com/coder/coder-desktop-macos/releases)
and run the installer. `Coder Desktop.app` is installed to your Applications
folder.

After launching the app, macOS will prompt you to allow a new network extension:

1. When prompted with **"Coder Desktop" would like to use a new network
   extension**, select **Open System Settings**.
2. In **Network Extensions** settings, enable the Coder Desktop extension.
3. Enter your password to authorize the extension if requested.

Coder Desktop appears in your menu bar when it is running.

### Windows

Install with WinGet:

```shell
winget install Coder.CoderDesktop
```

Or download the `CoderDesktop` installer (`.exe`) from the
[coder-desktop-windows releases page](https://github.com/coder/coder-desktop-windows/releases).
Pick the `x64` or `arm64` build to match your system, then run the installer
and accept the license terms.

Coder Desktop on Windows depends on:

- [.NET Windows Desktop Runtime](https://dotnet.microsoft.com/en-us/download/dotnet/8.0).
  The installer prompts you to install it if it is not already present.
- Windows App Runtime SDK. The installer may ask you to install this manually.

Coder Desktop appears in your system tray when it is running. Click the **^**
chevron if the icon is hidden.

### Linux (experimental)

A Linux client exists at
[coder/coder-desktop-linux](https://github.com/coder/coder-desktop-linux) but
is **experimental and not production ready**. There is no recommended install
command yet. To try it, download the latest build from the
[coder-desktop-linux releases page](https://github.com/coder/coder-desktop-linux/releases)
and follow the instructions in that repository.

If you need stable workspace access on Linux today, use the
[Coder CLI](../../install/cli.md).

## Get started

After installing Coder Desktop:

1. Open Coder Desktop and sign in with your deployment URL and session token.
2. Toggle **Coder Connect** on. Approve any system prompts to load the VPN
   extension.
3. Open `http://workspace-name.coder` in your browser, run
   `ssh workspace-name.coder`, or point any tool at `workspace-name.coder:PORT`.

That is the full setup. No SSH config, no `coder` CLI install, no per-port
forwarding rules.

## Test your connection

Once Coder Connect is enabled, confirm a workspace is reachable:

<div class="tabs">

### SSH Connection

```shell
ssh your-workspace.coder
```

### Ping Test

```shell
# macOS
ping6 -c 3 your-workspace.coder

# Windows
ping -n 3 your-workspace.coder
```

### Web Services

Open `http://your-workspace.coder:PORT` in your browser, replacing `PORT` with
the specific service port you want to access (e.g. 3000 for frontend, 8080 for
API).

</div>

## Administrator configuration

Organizations that manage Coder Desktop deployments can configure the
application using MDM (Mobile Device Management) or group policy.

### Disable automatic updates

Administrators can disable the built-in auto-updater to manage updates through
their own software distribution system.

<div class="tabs">

### macOS

Set the `disableUpdater` preference to `true` using the `defaults` command:

```shell
defaults write com.coder.Coder-Desktop disableUpdater -bool true
```

Organization administrators can also enforce this setting across managed
devices using MDM (Mobile Device Management) software by deploying a
configuration profile that sets this preference.

### Windows

Set the `Updater:Enable` registry value to `0` under
`HKEY_LOCAL_MACHINE\SOFTWARE\Coder Desktop\App`:

```powershell
New-Item -Path "HKLM:\SOFTWARE\Coder Desktop\App" -Force
New-ItemProperty -Path "HKLM:\SOFTWARE\Coder Desktop\App" -Name "Updater:Enable" -Value 0 -PropertyType DWord -Force
```

You can also configure a `Updater:ForcedChannel` string value to lock users to
a specific update channel (e.g. `stable`).

> [!NOTE]
> For security, updater settings can only be configured at the machine level
> (`HKLM`), not per-user (`HKCU`).

</div>

## Troubleshooting

### Connection Issues

#### Can't connect to workspace

- Verify Coder Connect is enabled (toggle should be ON)
- Check that your deployment URL is correct
- Ensure your session token hasn't expired
- Try disconnecting and reconnecting Coder Connect

#### VPN extension not working

- Restart Coder Desktop
- Check system permissions for network extensions
- Ensure only one copy of Coder Desktop is installed

### Getting help

If you encounter issues not covered here:

- **File an issue**:
  [macOS](https://github.com/coder/coder-desktop-macos/issues) |
  [Windows](https://github.com/coder/coder-desktop-windows/issues) |
  [General](https://github.com/coder/coder/issues)
- **Community support**: [Discord](https://coder.com/chat)

## Uninstall

<div class="tabs">

### macOS

1. **Disable Coder Connect** in the app menu
2. **Quit Coder Desktop** completely
3. **Remove VPN extension** from System Settings > Network Extensions
4. **Delete the app** from Applications folder
5. **Remove configuration** (optional):
   `rm -rf ~/Library/Application\ Support/Coder\ Desktop`

### Windows

1. **Disable Coder Connect** in the app menu
2. **Quit Coder Desktop** from system tray
3. **Uninstall** via Settings > Apps or Control Panel
4. **Remove configuration** (optional): Delete `%APPDATA%\Coder Desktop`

</div>

## Next steps

- [Using Coder Connect and File Sync](./desktop-connect-sync.md)
- [Compare port forwarding methods](../workspace-access/port-forwarding.md)
