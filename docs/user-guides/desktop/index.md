# Coder Desktop

Coder Desktop provides seamless access to your remote workspaces through a native application. Connect to workspace services using simple hostnames like `myworkspace.coder`, launch applications with one click, and synchronize files between local and remote environments—all without installing a CLI or configuring manual port forwarding.

## What You'll Need

- A Coder deployment running `v2.20.0` or [later](https://github.com/coder/coder/releases/latest)
- Administrator privileges on your local machine (for VPN extension installation)
- Access to your Coder deployment URL

## Quick Start

1. Install: `brew install --cask coder/coder/coder-desktop` (macOS) or `winget install Coder.CoderDesktop` (Windows)
1. Open Coder Desktop and approve any system prompts to complete the installation.
1. Sign in with your deployment URL and session token
1. Enable "Coder Connect" toggle
1. Access workspaces at `workspace-name.coder`

## How It Works

**Coder Connect**, the primary component of Coder Desktop, creates a secure tunnel to your Coder deployment, allowing you to:

- **Access workspaces directly**: Connect via `workspace-name.coder` hostnames
- **Use any application**: SSH clients, browsers, IDEs work seamlessly
- **Sync files**: Bidirectional sync between local and remote directories
- **Work offline**: Edit files locally, sync when reconnected

The VPN extension routes only Coder traffic—your other internet activity remains unchanged.

## Installation

<div class="tabs">

### macOS

<div class="tabs">

#### Homebrew (Recommended)

```shell
brew install --cask coder/coder/coder-desktop
```

#### Manual Installation

1. Download the latest release from [coder-desktop-macos releases](https://github.com/coder/coder-desktop-macos/releases)
1. Run `Coder-Desktop.pkg` and follow the prompts to install
1. `Coder Desktop.app` will be installed to your Applications folder

</div>

Coder Desktop requires VPN extension permissions:

1. When prompted with **"Coder Desktop" would like to use a new network extension**, select **Open System Settings**
1. In **Network Extensions** settings, enable the Coder Desktop extension
1. You may need to enter your password to authorize the extension

✅ **Verify Installation**: Coder Desktop should appear in your menu bar

### Windows

<div class="tabs">

#### WinGet (Recommended)

```shell
winget install Coder.CoderDesktop
```

#### Manual Installation

1. Download the latest `CoderDesktop` installer (`.exe`) from [coder-desktop-windows releases](https://github.com/coder/coder-desktop-windows/releases)
1. Choose the correct architecture (`x64` or `arm64`) for your system
1. Run the installer and accept the license terms
1. If prompted, install the .NET Windows Desktop Runtime
1. Install Windows App Runtime SDK if prompted

</div>

- [.NET Windows Desktop Runtime](https://dotnet.microsoft.com/en-us/download/dotnet/8.0) (installed automatically if not present)
- Windows App Runtime SDK (may require manual installation)

✅ **Verify Installation**: Coder Desktop should appear in your system tray (you may need to click **^** to show hidden icons)

</div>

## Testing Your Connection

Once connected, test access to your workspaces:

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

Open `http://your-workspace.coder:PORT` in your browser, replacing `PORT` with the specific service port you want to access (e.g. 3000 for frontend, 8080 for API)

</div>

## Administrator Configuration

Organizations that manage Coder Desktop deployments can configure the application using MDM (Mobile Device Management) or group policy.

### Disable Automatic Updates

Administrators can disable the built-in auto-updater to manage updates through their own software distribution system.

<div class="tabs">

### macOS

Set the `disableUpdater` preference to `true` using the `defaults` command:

```shell
defaults write com.coder.Coder-Desktop disableUpdater -bool true
```

Organization administrators can also enforce this setting across managed devices using MDM (Mobile Device Management) software by deploying a configuration profile that sets this preference.

When disabled, the "Check for Updates" option will not appear in the application menu.

### Windows

Set the `Updater:Enable` registry value to `0` under `HKEY_LOCAL_MACHINE\SOFTWARE\Coder Desktop\App`:

```powershell
New-Item -Path "HKLM:\SOFTWARE\Coder Desktop\App" -Force
New-ItemProperty -Path "HKLM:\SOFTWARE\Coder Desktop\App" -Name "Updater:Enable" -Value 0 -PropertyType DWord -Force
```

You can also configure a `Updater:ForcedChannel` string value to lock users to a specific update channel (e.g. `stable`).

> [!NOTE]
> For security, updater settings can only be configured at the machine level (`HKLM`), not per-user (`HKCU`).

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

### Getting Help

If you encounter issues not covered here:

- **File an issue**: [macOS](https://github.com/coder/coder-desktop-macos/issues) | [Windows](https://github.com/coder/coder-desktop-windows/issues) | [General](https://github.com/coder/coder/issues)
- **Community support**: [Discord](https://coder.com/chat)

## Uninstalling

<div class="tabs">

### macOS

1. **Disable Coder Connect** in the app menu
2. **Quit Coder Desktop** completely
3. **Remove VPN extension** from System Settings > Network Extensions
4. **Delete the app** from Applications folder
5. **Remove configuration** (optional): `rm -rf ~/Library/Application\ Support/Coder\ Desktop`

### Windows

1. **Disable Coder Connect** in the app menu
2. **Quit Coder Desktop** from system tray
3. **Uninstall** via Settings > Apps or Control Panel
4. **Remove configuration** (optional): Delete `%APPDATA%\Coder Desktop`

</div>

## Next Steps

- [Using Coder Connect and File Sync](./desktop-connect-sync.md)
