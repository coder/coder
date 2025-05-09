# JetBrains Toolbox Integration

JetBrains Toolbox helps you manage JetBrains products and includes remote development capabilities for connecting to Coder workspaces.

## Before you begin

- Install [JetBrains Toolbox](https://www.jetbrains.com/toolbox-app/) version 2.6.0.40284 or later
- Ensure your Coder workspace [has the necessary IDE backends installed](../../../admin/templates/extending-templates/jetbrains-gateway.md)

## Install the Coder plugin for Toolbox

1. Open Toolbox and navigate to the **Remote Development** section.
1. Install the Coder plugin using one of these methods:
   - Search for `Coder` in the **Remote Development** plugins section.
   - Use this URI to install directly: `jetbrains://gateway/com.coder.toolbox`.
   - Download from [JetBrains Marketplace](https://plugins.jetbrains.com/).

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

When connecting to a Coder deployment with internal certificates, follow the same procedure described in the [JetBrains Gateway](../index.md#configuring-the-gateway-plugin-to-use-internal-certificates) section above, but use the Toolbox installation paths:

<div class="tabs">

### Linux

```shell
keytool -import -alias coder -file <path-to-certificate> -keystore "<toolbox-installation>/jbr/lib/security/cacerts"
```

### macOS

```shell
keytool -import -alias coder -file <path-to-certificate> -keystore "$HOME/Library/Application Support/JetBrains/Toolbox/jbr/Contents/Home/lib/security/cacerts"
```

### Windows

```shell
keytool -import -alias coder -file <path-to-certificate> -keystore "%USERPROFILE%\AppData\Local\JetBrains\Toolbox\jbr\lib\security\cacerts"
```

</div>
