# JetBrains Gateway

JetBrains Gateway is a compact desktop app that allows you to work remotely with a JetBrains IDE without even downloading one. [See JetBrains' website to learn about and Gateway.](https://www.jetbrains.com/remote-development/gateway/)

Gateway can connect to a Coder workspace by using Coder's Gateway plugin or manually setting up an SSH connection.

## Using Coder's JetBrains Gateway Plugin

> If you experience problems, please [create a GitHub issue](https://github.com/coder/coder/issues) or share in [our Discord channel](https://discord.gg/coder).

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

> Note the JetBrains IDE is remotely installed into `~/.cache/JetBrains/RemoteDev/dist`

### Configuring the Gateway plugin to use internal certificates

When attempting to connect to a Coder deployment that uses internally signed certificates,
you may receive the following error in Gateway:

```console
Failed to configure connection to https://coder.internal.enterprise/: PKIX path building failed: sun.security.provider.certpath.SunCertPathBuilderException: unable to find valid certification path to requested target
```

To resolve this issue, you will need to add Coder's certificate to the Java trust store
present on your local machine. Here is the default location of the trust store for
each OS:

```console
# Linux
<Gateway installation directory>/jbr/lib/security/cacerts

# macOS
<Gateway installation directory>/jbr/lib/security/cacerts
/Library/Application Support/JetBrains/Toolbox/apps/JetBrainsGateway/ch-0/<app-id>/JetBrains Gateway.app/Contents/jbr/Contents/Home/lib/security/cacerts # Path for Toolbox installation

# Windows
C:\Program Files (x86)\<Gateway installation directory>\jre\lib\security\cacerts
%USERPROFILE%\AppData\Local\JetBrains\Toolbox\bin\jre\lib\security\cacerts # Path for Toolbox installation
```

To add the certificate to the keystore, you can use the `keytool` utility that ships
with Java:

```console
keytool -import -alias coder -file <certificate> -keystore /path/to/trust/store
```

You can use `keytool` that ships with the JetBrains Gateway installation.
Windows example:

```powershell
& 'C:\Program Files\JetBrains\JetBrains Gateway <version>/jbr/bin/keytool.exe' 'C:\Program Files\JetBrains\JetBrains Gateway <version>/jre/lib/security/cacerts' -import -alias coder -file <cert>

# command for Toolbox installation
& '%USERPROFILE%\AppData\Local\JetBrains\Toolbox\apps\Gateway\ch-0\<VERSION>\jbr\bin\keytool.exe' '%USERPROFILE%\AppData\Local\JetBrains\Toolbox\bin\jre\lib\security\cacerts' -import -alias coder -file <cert>
```

macOS example:

```sh
keytool -import -alias coder -file cacert.pem -keystore /Applications/JetBrains\ Gateway.app/Contents/jbr/Contents/Home/lib/security/cacerts
```

## Manually Configuring A JetBrains Gateway Connection

> This is in lieu of using Coder's Gateway plugin which automatically performs these steps.

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

## Using an existing JetBrains installation in the workspace

If you would like to use an existing JetBrains IDE in a Coder workspace (or you
are air-gapped, and cannot reach jetbrains.com), run the following script in the
JetBrains IDE directory to point the default Gateway directory to the IDE
directory. This step must be done before configuring Gateway.

```sh
cd /opt/idea/bin
./remote-dev-server.sh registerBackendLocationForGateway
```

> Gateway only works with paid versions of JetBrains IDEs so the script will not be located in the `bin` directory of JetBrains Community editions.

[Here is the JetBrains article](https://www.jetbrains.com/help/idea/remote-development-troubleshooting.html#setup:~:text=Can%20I%20point%20Remote%20Development%20to%20an%20existing%20IDE%20on%20my%20remote%20server%3F%20Is%20it%20possible%20to%20install%20IDE%20manually%3F)
explaining this IDE specification.
