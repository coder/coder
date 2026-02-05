# GitHub Copilot

[GitHub Copilot](https://github.com/features/copilot) is an AI coding assistant that doesn't support custom base URLs but does respect proxy configurations.
This makes it compatible with [AI Bridge Proxy](../ai-bridge-proxy/index.md), which integrates with [AI Bridge](../index.md) for full access to auditing and governance features.
To use Copilot with AI Bridge, make sure AI Bridge Proxy is properly configured, see [AI Bridge Proxy Setup](../ai-bridge-proxy/setup.md) for instructions.

Copilot uses **per-user tokens** tied to GitHub accounts rather than a shared API key.
Users must still authenticate with GitHub to use Copilot.

For general information about GitHub Copilot, see the [GitHub Copilot documentation](https://docs.github.com/en/copilot).

For general client configuration requirements, see [AI Bridge Proxy Client Configuration](../ai-bridge-proxy/setup.md#client-configuration).
The sections below cover Copilot-specific setup for each client.

## Copilot CLI

For installation instructions, see [GitHub Copilot CLI documentation](https://docs.github.com/en/copilot/how-tos/copilot-cli/install-copilot-cli).

### Proxy configuration

Set the `HTTP_PROXY` and `HTTPS_PROXY` environment variables:

```shell
export HTTP_PROXY="http://coder:${CODER_SESSION_TOKEN}@<proxy-host>:8888"
export HTTPS_PROXY="http://coder:${CODER_SESSION_TOKEN}@<proxy-host>:8888"
```

Replace `<proxy-host>` with your AI Bridge Proxy hostname.

### CA certificate trust

Copilot CLI is built on Node.js and uses the `NODE_EXTRA_CA_CERTS` environment variable for custom certificates:

```shell
export NODE_EXTRA_CA_CERTS="/path/to/aiproxy-ca.pem"
```

See [Client Configuration CA certificate trust](../ai-bridge-proxy/setup.md#trusting-the-ca-certificate) for details on how to obtain the certificate file.

## VS Code Copilot Extension

For installation instructions, see [Installing the GitHub Copilot extension in VS Code](https://docs.github.com/en/copilot/how-tos/set-up/install-copilot-extension?tool=vscode).

### Proxy configuration

You can configure the proxy using environment variables or VS Code settings.
For environment variables, see [AI Bridge Proxy client configuration](../ai-bridge-proxy/setup.md#configuring-the-proxy).

Alternatively, you can configure the proxy directly in VS Code settings:

1. Open Settings (`Ctrl+,` for Windows or `Cmd+,` for macOS)
1. Search for `HTTP: Proxy`
1. Set the proxy URL using the format `http://coder:<CODER_SESSION_TOKEN>@<proxy-host>:8888`

Or add directly to your `settings.json`:

```json
{
    "http.proxy": "http://coder:<CODER_SESSION_TOKEN>@<proxy-host>:8888"
}
```

The `http.proxy` setting is used for both HTTP and HTTPS requests.
Replace `<proxy-host>` with your AI Bridge Proxy hostname and `<CODER_SESSION_TOKEN>` with your coder session token.

Restart VS Code for changes to take effect.

For more details, see [Configuring proxy settings for Copilot](https://docs.github.com/en/copilot/how-tos/configure-personal-settings/configure-network-settings?tool=vscode) in the GitHub documentation.

### CA certificate trust

Add the AI Bridge Proxy CA certificate to your operating system's trust store.
By default, VS Code loads system certificates, controlled by the `http.systemCertificates` setting.

See [Client Configuration CA certificate trust](../ai-bridge-proxy/setup.md#trusting-the-ca-certificate) for details on how to obtain the certificate file.

### Using Coder Remote extension

When connecting to a Coder workspace with the [Coder extension](https://marketplace.visualstudio.com/items?itemName=coder.coder-remote), the Copilot extension runs inside the Coder workspace and not on your local machine.
This means proxy and certificate configuration must be done in the Coder workspace environment.

#### Proxy configuration

Configure the proxy in VS Code's remote settings:

1. [Connect to your Coder workspace](../../../user-guides/workspace-access/vscode.md)
1. Open Settings (`Ctrl+,` for Windows or `Cmd+,` for macOS)
1. Select the **Remote** tab
1. Search for `HTTP: Proxy`
1. Set the proxy URL using the format `http://coder:<CODER_SESSION_TOKEN>@<proxy-host>:8888`

Replace `<proxy-host>` with your AI Bridge Proxy hostname and `<CODER_SESSION_TOKEN>` with your coder session token.

#### CA certificate trust

Since the Copilot extension runs inside the Coder workspace, add the [AI Bridge Proxy CA certificate](../ai-bridge-proxy/setup.md#trusting-the-ca-certificate) to the Coder workspace's system trust store.
See [System trust store](../ai-bridge-proxy/setup.md#system-trust-store) for instructions on how to do this on Linux.

Restart VS Code for changes to take effect.

## JetBrains IDEs

For installation instructions, see [Installing the GitHub Copilot extension in JetBrains IDE](https://docs.github.com/en/copilot/how-tos/set-up/install-copilot-extension?tool=jetbrains).

### Proxy configuration

Configure the proxy directly in JetBrains IDE settings:

1) Open Settings (`Ctrl+Alt+S` for Windows or `Cmd+,` for macOS)
1) Navigate to `Appearance & Behavior` > `System Settings` > `HTTP Proxy`
1) Select `Manual proxy configuration` and `HTTP`
1) Enter the proxy hostname and port (default: 8888)
1) Select `Proxy authentication` and enter:
   1) Login: `coder` (this value is ignored)
   1) Password: Your Coder session token
   1) Check `Remember` to save the password
1) Restart the IDE for changes to take effect

For more details, see [Configuring proxy settings for Copilot](https://docs.github.com/en/copilot/how-tos/configure-personal-settings/configure-network-settings?tool=jetbrains) in the GitHub documentation.

### CA certificate trust

Add the AI Bridge Proxy CA certificate to your operating system's trust store.
If the certificate is in the system trust store, no additional IDE configuration is needed.

Alternatively, you can configure the IDE to accept the certificate:

1) Open Settings (`Ctrl+Alt+S` for Windows or `Cmd+,` for macOS)
1) Navigate to `Appearance & Behavior` > `System Settings` > `Server Certificates`
1) Under `Accepted certificates`, click `+` and select the CA certificate file
1) Check `Accept non-trusted certificates automatically`
1) Restart the IDE for changes to take effect

For more details, see [Trusted root certificates](https://www.jetbrains.com/help/idea/ssl-certificates.html) in the JetBrains documentation.

See [Client Configuration CA certificate trust](../ai-bridge-proxy/setup.md#trusting-the-ca-certificate) for details on how to obtain the certificate file.
