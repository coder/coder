# JetBrains Toolbox Integration

JetBrains Toolbox helps you manage JetBrains products and includes remote development capabilities for connecting to Coder workspaces.

## Install the Coder plugin for Toolbox

1. Install [JetBrains Toolbox](https://www.jetbrains.com/toolbox-app/) version 2.6.0.40632 or later.

1. Open Toolbox and navigate to the **Remote Development** section.
1. Install the Coder plugin using one of these methods:
   - Search for `Coder` in the **Remote Development** plugins section.
   - Use this URI to install directly: `jetbrains://gateway/com.coder.toolbox`.
   - Download from [JetBrains Marketplace](https://plugins.jetbrains.com/).
   - Download from [GitHub Releases](https://github.com/coder/coder-jetbrains-toolbox/releases).

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

To connect to a Coder deployment that uses internal certificates, configure the certificates directly in JetBrains Toolbox:

1. Click the settings icon (⚙) in the lower left corner of JetBrains Toolbox.
1. Select **Settings**.
1. Go to the **Coder** section.
1. Add your certificate path in the **CA Path** field.
