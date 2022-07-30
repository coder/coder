# Configuring JetBrains Gateway

The following screenshots outline in more detail how to create a new Connection
for JetBrains Gateway to a Coder workspace.

## Creating a new JetBrains Gateway Connection

After installing Gateway on your local system, you may connect to a Coder
workspace as follows

1. Open Gateway, make sure "SSH" is selected under "Remote Development"
2. Click "New Connection"

![Gateway Home](../images/gateway/gateway-home.png)

3. In the resulting dialog, click the gear icon to the right of "Connection:"

![Gateway New Connection](../images/gateway/gateway-new-connection.png)

4. Hit the "+" button to add a new SSH connection

![Gateway Add Connection](../images/gateway/gateway-add-ssh-configuration.png)

   1. For the Host, enter `coder.<workspace name>`
   2. For the Port, enter `22` (this is ignored by Coder)
   3. For the Username, enter `coder`
   4. For the Authentication Type, select "OpenSSH config and authentication
      agent"
   5. Make sure the checkbox for "Parse config file ~/.ssh/config" is checked.
   6. Click "Test Connection" to ensure you setting are ok.
   7. Click "OK"

![Gateway SSH
Configuration](../images/gateway/gateway-create-ssh-configuration.png)

5. Select the connection you just added.

![Gateway Welcome](../images/gateway/gateway-welcome.png)

6. Click "Check Connection and Continue"

![Gateway Continue](../images/gateway/gateway-continue.png)

7. Select the JetBrains IDE for your project and the project directory
   1. Use an SSH terminal to your workspace to create a directory or check out
      code if you haven't already.

> Note the JetBrains IDE is installed in the directory `~/.
> cache/JetBrains/RemoteDev/dist`

![Gateway Choose IDE](../images/gateway/gateway-choose-ide.png)

8. Click "Download and Start IDE" to connect.

![Gateway IDE Opened](../images/gateway/gateway-intellij-opened.png)


