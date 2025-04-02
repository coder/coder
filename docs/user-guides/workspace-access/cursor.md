# Cursor

[Cursor](https://cursor.sh/) is a modern IDE built on top of VS Code with enhanced AI capabilities.

Follow this guide to use Cursor to access your Coder workspaces.

If your team uses Cursor regularly, ask your Coder administrator to add a [Cursor module](https://registry.coder.com/modules/cursor) to your template.

## Install Cursor

Cursor can connect to a Coder workspace using the Coder extension:

1. [Install Cursor](https://docs.cursor.com/get-started/installation) on your local machine.

1. Open Cursor and log in or [create a Cursor account](https://authenticator.cursor.sh/sign-up)
   if you don't have one already.

## Install the Coder extension

1. You can install the Coder extension through the Marketplace built in to Cursor or manually.

   <div class="tabs">

   ## Extension Marketplace

   1. Search for Coder from the Extensions Pane and select **Install**.

   1. Coder Remote uses the **Remote - SSH extension** to connect.

      You can find it in the **Extension Pack** tab of the Coder extension.

   ## Manually

   1. Download the [latest vscode-coder extension](https://github.com/coder/vscode-coder/releases/latest) `.vsix` file.

   1. Drag the `.vsix` file into the extensions pane of Cursor.

      Alternatively:

      1. Open the Command Palette
   (<kdb>Ctrl</kdb>+<kdb>Shift</kdb>+<kdb>P</kdb> or <kdb>Cmd</kdb>+<kdb>Shift</kdb>+<kdb>P</kdb>)
   and search for `vsix`.

      1. Select **Extensions: Install from VSIX** and select the vscode-coder extension you downloaded.

   </div>

1. Coder Remote uses the **Remote - SSH extension** to connect.

   You can find it in the **Extension Pack** tab of the Coder extension.

## Open a workspace in Cursor

1. From the Cursor Command Palette
(<kdb>Ctrl</kdb>+<kdb>Shift</kdb>+<kdb>P</kdb> or <kdb>Cmd</kdb>+<kdb>Shift</kdb>+<kdb>P</kdb>),
enter `coder` and select **Coder: Login**.

1. Follow the prompts to login and copy your session token.

   Paste the session token in the **Paste your API key** box in Cursor.

1. Select **Open Workspace** or use the Command Palette to run **Coder: Open Workspace**.
