# Antigravity

[Antigravity](https://antigravity.google/) is Google's desktop IDE.

Follow this guide to use Antigravity to access your Coder workspaces.

If your team uses Antigravity regularly, ask your Coder administrator to add Antigravity as a workspace application in your template.
You can also use the [Antigravity module](https://registry.coder.com/modules/coder/antigravity) to easily add Antigravity to your Coder templates.

## Install Antigravity

Antigravity connects to your Coder workspaces using the Coder extension:

1. [Install Antigravity](https://antigravity.google/) on your local machine.

1. Open Antigravity and sign in with your Google account.

## Install the Coder extension

1. You can install the Coder extension through the Marketplace built in to Antigravity or manually.

   <div class="tabs">

   ## Extension Marketplace

   Search for Coder from the Extensions Pane and select **Install**.

   ## Manually

   1. Download the [latest vscode-coder extension](https://github.com/coder/vscode-coder/releases/latest) `.vsix` file.

   1. Drag the `.vsix` file into the extensions pane of Antigravity.

      Alternatively:

      1. Open the Command Palette
         (<kdb>Ctrl</kdb>+<kdb>Shift</kdb>+<kdb>P</kdb> or <kdb>Cmd</kdb>+<kdb>Shift</kdb>+<kdb>P</kdb>) and search for `vsix`.

      1. Select **Extensions: Install from VSIX** and select the vscode-coder extension you downloaded.

   </div>

## Open a workspace in Antigravity

1. From the Antigravity Command Palette (<kdb>Ctrl</kdb>+<kdb>Shift</kdb>+<kdb>P</kdb> or <kdb>Cmd</kdb>+<kdb>Shift</kdb>+<kdb>P</kdb>),
   enter `coder` and select **Coder: Login**.

1. Follow the prompts to login and copy your session token.

   Paste the session token in the **Coder API Key** dialogue in Antigravity.

1. Antigravity prompts you to open a workspace, or you can use the Command Palette to run **Coder: Open Workspace**.

## Template configuration

Your Coder administrator can add Antigravity as a one-click workspace app using
the [Antigravity module](https://registry.coder.com/modules/coder/antigravity)
from the Coder registry:

```tf
module "antigravity" {
  count    = data.coder_workspace.me.start_count
  source   = "registry.coder.com/coder/antigravity/coder"
  version  = "1.0.0"
  agent_id = coder_agent.example.id
  folder   = "/home/coder/project"
}
```
