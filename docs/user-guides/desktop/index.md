# Coder Desktop

Use Coder Desktop to work on your workspaces as though they're on your LAN, no
port-forwarding required.

## Install Coder Desktop

Coder Desktop is available for early access testing on macOS.
A Windows version is in development.

### macOS

1. Use [Homebrew](https://brew.sh/) to install Coder Desktop:

   ```shell
   brew install --cask coder/coder/coder-desktop
   ```

1. Open Coder Desktop from the Applications directory and when macOS asks if you want to open it, select **Open**.

1. The application is treated as a VPN. macOS will prompt you to confirm with:

   **"Coder Desktop" would like to use a new network extension**

   Select **Open System Settings**.

1. In the **Network Extensions** system settings, enable the Coder Desktop extension.

1. Continue to the [configuration section](#configure).

## Configure

Before you can use Coder Desktop, you will need to log in.

1. Open the Desktop menu again and select **Sign in**:

   <Image height="325px" src="../../images/user-guides/desktop/coder-desktop-pre-sign-in.png" alt="Coder Desktop menu before the user signs in" align="center" />

1. In the **Sign In** window, enter your Coder deployment's URL and select **Next**:

   ![Coder Desktop sign in](../../images/user-guides/desktop/coder-desktop-sign-in.png)

1. Select the link to your deployment's `/cli-auth` page to generate a [session token](../../admin/users/sessions-tokens.md).

1. In your web browser, enter your credentials:

   <Image height="412px" src="../../images/templates/coder-login-web.png" alt="Log in to your Coder deployment" align="center" />

1. Copy the session token to the clipboard:

   <Image height="350px" src="../../images/templates/coder-session-token.png" alt="Copy session token" align="center" />

1. Paste the token in the **Session Token** box of the **Sign In** screen, then select **Sign In**:

   ![Paste the session token in to sign in](../../images/user-guides/desktop/coder-desktop-session-token.png)

1. Allow the VPN configuration for Coder Desktop if your OS prompts you.

1. Coder Desktop is now running!

   Select the Coder icon in the menu bar to enable [CoderVPN](#codervpn).

## Workspaces from Coder Desktop

You can use `ping6` in your terminal to verify the connection to your workspace:

```shell
ping6 -c 5 yourworkspacename.coder
```

## CoderVPN

Placeholder for some information about CoderVPN
