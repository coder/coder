# JetBrains Gateway

JetBrains Gateway is a compact desktop app that allows you to work remotely with a JetBrains IDE without even downloading one. [See JetBrains' website to learn about and Gateway.](https://www.jetbrains.com/remote-development/gateway/)

Gateway can connect to a Coder workspace by using Coder's Gateway plugin or manually setting up an SSH connection.

## Using Coder's JetBrains Gateway Plugin

> The Coder plugin is an alpha state. If you experience problems, please [create a GitHub issue](https://github.com/coder/coder/issues) or share in [our Discord channel](https://discord.gg/coder).

1. [Install Gateway](https://www.jetbrains.com/help/idea/jetbrains-gateway.html)
1. Open Gateway and click the gear icon at the bottom left and then "Settings"
1. In the Marketplace tab within Plugins, type Coder and then click "Install" and "OK"
   ![Gateway Settings and Marketplace](../images/gateway/plugin-settings-marketplace.png)
1. Click the new "Coder" icon on the Gateway home screen
   ![Gateway Connect to Coder](../images/gateway/plugin-connect-to-coder.png)
1. Enter your Coder deployment's Access Url and click "Connect" then paste the Session Token and click "OK"
   ![Gateway Session Token](../images/gateway/plugin-session-token.png)
1. Click the "+" icon to open a browser and go to the templates page in your Coder deployment to create a workspace
1. If a workspace already exists but is stopped, click the green arrow to start the workspace
1. Once the workspace status says Running, click "Select IDE and Project"
   ![Gateway IDE List](../images/gateway/plugin-select-ide.png)
1. Select the JetBrains IDE for your project and the project directory then click "Start IDE and connect"
   ![Gateway Select IDE](../images/gateway/plugin-ide-list.png)
   ![Gateway IDE Opened](../images/gateway/gateway-intellij-opened.png)

> Note the JetBrains IDE is remotely installed into `~/. cache/JetBrains/RemoteDev/dist`

## Creating a new JetBrains Gateway Connection

1. [Install Gateway](https://www.jetbrains.com/help/idea/jetbrains-gateway.html)
1. [Configure the `coder` CLI](../ides.md#ssh-configuration)
1. Open Gateway, make sure "SSH" is selected under "Remote Development"
1. Click "New Connection"
   ![Gateway Home](../images/gateway/gateway-home.png)
1. In the resulting dialog, click the gear icon to the right of "Connection:"
   ![Gateway New Connection](../images/gateway/gateway-new-connection.png)
1. Hit the "+" button to add a new SSH connection
   ![Gateway Add Connection](../images/gateway/gateway-add-ssh-configuration.png)

1. For the Host, enter `coder.<workspace name>`
1. For the Port, enter `22` (this is ignored by Coder)
1. For the Username, enter your workspace username
1. For the Authentication Type, select "OpenSSH config and authentication
   agent"
1. Make sure the checkbox for "Parse config file ~/.ssh/config" is checked.
1. Click "Test Connection" to validate these settings.
1. Click "OK"
   ![Gateway SSH Configuration](../images/gateway/gateway-create-ssh-configuration.png)
1. Select the connection you just added
   ![Gateway Welcome](../images/gateway/gateway-welcome.png)
1. Click "Check Connection and Continue"
   ![Gateway Continue](../images/gateway/gateway-continue.png)
1. Select the JetBrains IDE for your project and the project directory.
   SSH into your server to create a directory or check out code if you haven't already.
   ![Gateway Choose IDE](../images/gateway/gateway-choose-ide.png)
   > Note the JetBrains IDE is remotely installed into `~/. cache/JetBrains/RemoteDev/dist`
1. Click "Download and Start IDE" to connect.
   ![Gateway IDE Opened](../images/gateway/gateway-intellij-opened.png)
