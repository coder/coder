# JetBrains Gateway

The following walkthrough details how to connect JetBrains Gateway to
Coder.

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
