# JetBrains Toolbox (beta)

JetBrains Toolbox helps you manage JetBrains products and includes remote development capabilities for connecting to Coder workspaces.

For more details, visit the [official JetBrains documentation](https://www.jetbrains.com/help/toolbox-app/manage-providers.html#shx3a8_18).

## Install the Coder provider for Toolbox

1. Install [JetBrains Toolbox](https://www.jetbrains.com/toolbox-app/) version 2.6.0.40632 or later.
1. Open the Toolbox App.
1. From the switcher drop-down, select **Manage Providers**.
1. In the **Providers** window, under the Available node, locate the **Coder** provider and click **Install**.

![Install the Coder provider in JetBrains Toolbox](../../../images/user-guides/jetbrains/toolbox/install.png)

## Connect

1. In the Toolbox App, click **Coder**.
1. Enter the URL address and click **Sign In**.
   ![JetBrains Toolbox Coder provider URL](../../../images/user-guides/jetbrains/toolbox/login-url.png)
1. Authenticate to Coder adding a token for the session and click **Connect**.
   ![JetBrains Toolbox Coder provider token](../../../images/user-guides/jetbrains/toolbox/login-token.png)
   After the authentication is completed, you are connected to your development environment and can open and work on projects.
   ![JetBrains Toolbox Coder Workspaces](../../../images/user-guides/jetbrains/toolbox/workspaces.png)

## Use URI parameters

For direct connections or creating bookmarks, use custom URI links with parameters:

```shell
jetbrains://gateway/com.coder.toolbox?url=https://coder.example.com&token=<auth-token>&workspace=my-workspace
```

Required parameters:

- `url`: Your Coder deployment URL
- `token`: Coder authentication token
- `workspace`: Name of your workspace

Optional parameters:

- `agent_id`: ID of the agent (only required if workspace has multiple agents)
- `folder`: Specific project folder path to open
- `ide_product_code`: Specific IDE product code (e.g., "IU" for IntelliJ IDEA Ultimate)
- `ide_build_number`: Specific build number of the JetBrains IDE

For more details, see the [coder-jetbrains-toolbox repository](https://github.com/coder/coder-jetbrains-toolbox#connect-to-a-coder-workspace-via-jetbrains-toolbox-uri).

## Configure internal certificates

To connect to a Coder deployment that uses internal certificates, configure the certificates directly in the Coder plugin settings in JetBrains Toolbox:

1. In the Toolbox App, click **Coder**.
1. Click the (⋮) next to the username in top right corner.
1. Select **Settings**.
1. Add your certificate path in the **CA Path** field.
   ![JetBrains Toolbox Coder Provider certificate path](../../../images/user-guides/jetbrains/toolbox/certificate.png)
