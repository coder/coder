# Claude Desktop

The Code tab in Claude Desktop runs the Claude Code engine and makes API
calls to `api.anthropic.com`.
[AI Gateway Proxy](../ai-gateway-proxy/index.md) intercepts this traffic
transparently, so users keep their normal Anthropic login and conversation
history while AI Gateway provides governance and observability.

To use Claude Desktop with AI Gateway, make sure AI Gateway Proxy is
configured. See [AI Gateway Proxy Setup](../ai-gateway-proxy/setup.md) for
instructions.

> [!NOTE]
> Only the **Code tab** is supported. The Cowork tab runs inside an
> isolated VM that does not inherit proxy settings. Proxy support for
> Cowork is tracked in
> [anthropics/claude-code#45994](https://github.com/anthropics/claude-code/issues/45994).

## Prerequisites

- Claude Desktop installed ([download](https://claude.com/download))
- [AI Gateway Proxy](../ai-gateway-proxy/setup.md) enabled on your Coder
  deployment
- A **[Coder API token](../../../admin/users/sessions-tokens.md#generate-a-long-lived-api-token-on-behalf-of-yourself)**
  for authentication with AI Gateway

## Proxy configuration

Claude Desktop is an Electron app that does not read shell profile files
(`~/.zshrc`, `~/.bashrc`). You must set the `HTTPS_PROXY` environment
variable in a way that the desktop application can see it.

Developer Mode is **not** required. The proxy operates at the OS network
level, so no in-app configuration is needed.

> [!NOTE]
> If [TLS is not enabled](../ai-gateway-proxy/setup.md#proxy-tls-configuration)
> on the proxy, replace `https://` with `http://` in the proxy URL.

Replace `<proxy-host>` with your AI Gateway Proxy hostname and
`<your-coder-api-token>` with your Coder API token in the examples below.

<div class="tabs">

### macOS

Use `launchctl setenv` to inject the variable into the GUI application
environment. This only affects applications launched after the command
runs, so quit and reopen Claude Desktop afterwards.

```sh
launchctl setenv HTTPS_PROXY "https://coder:<your-coder-api-token>@<proxy-host>:8888"
```

To persist across reboots, add the command to a login script or
LaunchAgent.

To remove the proxy setting:

```sh
launchctl unsetenv HTTPS_PROXY
```

### Windows

Set a user-level environment variable so the desktop application picks it
up on next launch. Open PowerShell and run:

```powershell
[Environment]::SetEnvironmentVariable(
  "HTTPS_PROXY",
  "https://coder:<your-coder-api-token>@<proxy-host>:8888",
  "User"
)
```

Quit and reopen Claude Desktop for the change to take effect.

To remove the proxy setting:

```powershell
[Environment]::SetEnvironmentVariable("HTTPS_PROXY", $null, "User")
```

### Linux

Set the variable before launching Claude Desktop from the terminal:

```sh
HTTPS_PROXY="https://coder:<your-coder-api-token>@<proxy-host>:8888" claude-desktop
```

For `.desktop` file launchers, add the variable to the `Exec` line or
create a wrapper script that exports it before starting the application.

</div>

## CA certificate trust

Claude Desktop must trust the AI Gateway Proxy CA certificate. Add the
certificate to your operating system's trust store so the Electron app
picks it up automatically.

See [Trusting the CA certificate](../ai-gateway-proxy/setup.md#trusting-the-ca-certificate)
for how to download the certificate, and
[System trust store](../ai-gateway-proxy/setup.md#system-trust-store) for
platform-specific instructions.

When [TLS is enabled](../ai-gateway-proxy/setup.md#proxy-tls-configuration)
on the proxy, add the TLS certificate to the system trust store as well.

## Claude Code CLI

Claude Desktop and the Claude Code CLI read their own configuration
independently. If you also use Claude Code in the terminal, you can either:

- Configure the CLI to use AI Gateway Proxy with the same `HTTPS_PROXY`
  and CA certificate settings described above.
- Configure the CLI to use AI Gateway directly with
  [environment variables](./claude-code.md), which does not require the
  proxy.

**References:**
[Claude Desktop documentation](https://code.claude.com/docs/en/desktop),
[AI Gateway Proxy Setup](../ai-gateway-proxy/setup.md),
[Claude Code network configuration](https://code.claude.com/docs/en/network-config)
