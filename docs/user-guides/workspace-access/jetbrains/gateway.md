## JetBrains Gateway

> [!WARNING]
> Using Coder through JetBrains Gateway is not recommended at this time. Instead, we suggest using [JetBrains Toolbox](https://coder.com/docs/user-guides/workspace-access/jetbrains/toolbox) for stability and performance benefits. If you are currently using Gateway, we recommend [migration](https://www.jetbrains.com/help/toolbox-app/jetbrains-gateway-migrations-guide.html).

JetBrains Gateway is a compact desktop app that allows you to work remotely with
a JetBrains IDE without downloading one. Visit the
[JetBrains Gateway website](https://www.jetbrains.com/remote-development/gateway/)
to learn more about Gateway.

Gateway can connect to a Coder workspace using Coder's Gateway plugin or through a
manually configured SSH connection.

### How to use the plugin

> [!NOTE]
> If you experience problems, please
> [create a GitHub issue](https://github.com/coder/coder/issues) or share in
> [our Discord channel](https://discord.gg/coder).

1. [Install Gateway](https://www.jetbrains.com/help/idea/jetbrains-gateway.html)
   and open the application.
1. Under **Install More Providers**, find the Coder icon and select **Install**
   to install the Coder plugin.
1. After Gateway installs the plugin, it will appear in the **Run the IDE
   Remotely** section.

   Select **Connect to Coder** to launch the plugin:

   ![Gateway Connect to Coder](../../../images/gateway/plugin-connect-to-coder.png)

1. Enter your Coder deployment's
   [Access Url](../../../admin/setup/index.md#access-url) and select **Connect**.

   Gateway opens your Coder deployment's `cli-auth` page with a session token.
   Select the copy button, paste the session token in the Gateway **Session
   Token** window, then select **OK**:

   ![Gateway Session Token](../../../images/gateway/plugin-session-token.png)

1. To create a new workspace:

   Select the <kbd>+</kbd> icon to open a browser and go to the templates page in
   your Coder deployment to create a workspace.

1. If a workspace already exists but is stopped, select the workspace from the
   list, then select the green arrow to start the workspace.

1. When the workspace status is **Running**, select **Select IDE and Project**:

   ![Gateway IDE List](../../../images/gateway/plugin-select-ide.png)

1. Select the JetBrains IDE for your project and the project directory then
   select **Start IDE and connect**:

   ![Gateway Select IDE](../../../images/gateway/plugin-ide-list.png)

   Gateway connects using the IDE you selected:

   ![Gateway IDE Opened](../../../images/gateway/gateway-intellij-opened.png)

   The JetBrains IDE is remotely installed into `~/.cache/JetBrains/RemoteDev/dist`.

### Update a Coder plugin version

1. Select the gear icon at the bottom left of the Gateway home screen, then
   **Settings**.

1. In the **Marketplace** tab within Plugins, enter Coder and if a newer plugin
   release is available, select **Update** then **OK**:

   ![Gateway Settings and Marketplace](../../../images/gateway/plugin-settings-marketplace.png)

### Configuring the Gateway plugin to use internal certificates

When you attempt to connect to a Coder deployment that uses internally signed
certificates, you might receive the following error in Gateway:

```console
Failed to configure connection to https://coder.internal.enterprise/: PKIX path building failed: sun.security.provider.certpath.SunCertPathBuilderException: unable to find valid certification path to requested target
```

To resolve this issue, you will need to add Coder's certificate to the Java
trust store present on your local machine as well as to the Coder plugin settings.

1. Add the certificate to the Java trust store:

   <div class="tabs">

   #### Linux

   ```txt
   <Gateway installation directory>/jbr/lib/security/cacerts
   ```

   Use the `keytool` utility that ships with Java:

   ```sh
   keytool -import -alias coder -file <certificate> -keystore /path/to/trust/store
   ```

   #### macOS

   ```txt
   <Gateway installation directory>/jbr/lib/security/cacerts
   /Library/Application Support/JetBrains/Toolbox/apps/JetBrainsGateway/ch-0/<app-id>/JetBrains Gateway.app/Contents/jbr/Contents/Home/lib/security/cacerts # Path for Toolbox installation
   ```

   Use the `keytool` included in the JetBrains Gateway installation:

   ```sh
   keytool -import -alias coder -file cacert.pem -keystore /Applications/JetBrains\ Gateway.app/Contents/jbr/Contents/Home/lib/security/cacerts
   ```

   #### Windows

   ```txt
   C:\Program Files (x86)\<Gateway installation directory>\jre\lib\security\cacerts\%USERPROFILE%\AppData\Local\JetBrains\Toolbox\bin\jre\lib\security\cacerts # Path for Toolbox installation
   ```

   Use the `keytool` included in the JetBrains Gateway installation:

   ```ps1
   & 'C:\Program Files\JetBrains\JetBrains Gateway <version>/jbr/bin/keytool.exe' 'C:\Program Files\JetBrains\JetBrains Gateway <version>/jre/lib/security/cacerts' -import -alias coder -file <cert>

   # command for Toolbox installation
   & '%USERPROFILE%\AppData\Local\JetBrains\Toolbox\apps\Gateway\ch-0\<VERSION>\jbr\bin\keytool.exe' '%USERPROFILE%\AppData\Local\JetBrains\Toolbox\bin\jre\lib\security\cacerts' -import -alias coder -file <cert>
   ```

   </div>

1. In JetBrains, go to **Settings** > **Tools** > **Coder**.

1. Paste the path to the certificate in **CA Path**.

## Manually Configuring A JetBrains Gateway Connection

This is in lieu of using Coder's Gateway plugin which automatically performs these steps.

1. [Install Gateway](https://www.jetbrains.com/help/idea/jetbrains-gateway.html).

1. [Configure the `coder` CLI](../index.md#configure-ssh).

1. Open Gateway, make sure **SSH** is selected under **Remote Development**.

1. Select **New Connection**:

   ![Gateway Home](../../../images/gateway/gateway-home.png)

1. In the resulting dialog, select the gear icon to the right of **Connection**:

   ![Gateway New Connection](../../../images/gateway/gateway-new-connection.png)

1. Select <kbd>+</kbd> to add a new SSH connection:

   ![Gateway Add Connection](../../../images/gateway/gateway-add-ssh-configuration.png)

1. For the Host, enter `coder.<workspace name>`

1. For the Port, enter `22` (this is ignored by Coder)

1. For the Username, enter your workspace username.

1. For the Authentication Type, select **OpenSSH config and authentication
   agent**.

1. Make sure the checkbox for **Parse config file ~/.ssh/config** is checked.

   > [!TIP]
   > Gateway discovers hosts by parsing `~/.ssh/config`. If your workspaces
   > do not appear in Gateway's host list, re-run `coder config-ssh
   > --no-wildcard` to generate an individual `Host` entry per workspace
   > instead of a wildcard block.

1. Select **Test Connection** to validate these settings.

1. Select **OK**:

   ![Gateway SSH Configuration](../../../images/gateway/gateway-create-ssh-configuration.png)

1. Select the connection you just added:

   ![Gateway Welcome](../../../images/gateway/gateway-welcome.png)

1. Select **Check Connection and Continue**:

   ![Gateway Continue](../../../images/gateway/gateway-continue.png)

1. Select the JetBrains IDE for your project and the project directory. SSH into
   your server to create a directory or check out code if you haven't already.

   ![Gateway Choose IDE](../../../images/gateway/gateway-choose-ide.png)

   The JetBrains IDE is remotely installed into `~/.cache/JetBrains/RemoteDev/dist`

1. Select **Download and Start IDE** to connect.

   ![Gateway IDE Opened](../../../images/gateway/gateway-intellij-opened.png)

## Using an existing JetBrains installation in the workspace

You can ask your template administrator to [pre-install the JetBrains IDEs backend](../../../admin/templates/extending-templates/jetbrains-preinstall.md) in a template to make JetBrains IDE start faster on first connection.
