# Claude Desktop

Claude Desktop's Code tab runs the Claude Code engine and makes API calls to
`api.anthropic.com`. [AI Gateway Proxy](../ai-gateway-proxy/index.md)
intercepts this traffic transparently, so users keep their normal Anthropic
login, conversation history, and the full Claude Desktop experience (Chat,
Cowork, and Code tabs) while AI Gateway provides governance and observability.

To use Claude Desktop with AI Gateway, make sure AI Gateway Proxy is
configured. See [AI Gateway Proxy Setup](../ai-gateway-proxy/setup.md) for
instructions.

For general client configuration requirements, see
[AI Gateway Proxy Client Configuration](../ai-gateway-proxy/setup.md#client-configuration).

## Prerequisites

- Claude Desktop installed ([download](https://claude.com/download))
- [AI Gateway Proxy](../ai-gateway-proxy/setup.md) enabled on your Coder
  deployment
- A **[Coder API token](../../../admin/users/sessions-tokens.md#generate-a-long-lived-api-token-on-behalf-of-yourself)**
  for authentication with AI Gateway

## Proxy configuration

Claude Desktop respects system proxy settings. Configure the proxy so that
traffic to `api.anthropic.com` is routed through AI Gateway Proxy.

### Environment variable

```bash
export HTTPS_PROXY="https://coder:<your-coder-api-token>@<proxy-host>:8888"
```

Replace `<proxy-host>` with your AI Gateway Proxy hostname.

> [!NOTE]
> If [TLS is not enabled](../ai-gateway-proxy/setup.md#proxy-tls-configuration)
> on the proxy, replace `https://` with `http://` in the proxy URL.

### System proxy (alternative)

You can also configure the proxy at the OS level so Claude Desktop picks it
up automatically:

- **macOS**: System Settings > Network > your connection > Details > Proxies.
- **Windows**: Settings > Network & Internet > Proxy > Manual proxy setup.

Enter the proxy host, port, and credentials (username: `coder`, password:
your Coder API token).

## CA certificate trust

Claude Desktop must trust the AI Gateway Proxy CA certificate. Add the
certificate to your operating system's trust store.

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
[AI Gateway Proxy Setup](../ai-gateway-proxy/setup.md)
