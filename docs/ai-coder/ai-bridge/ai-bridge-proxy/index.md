# AI Bridge Proxy

AI Bridge Proxy provides a way to route AI traffic through Bridge for tools that
**do not support base URL overrides**. While most AI tools allow customizing the
API base URL (the [recommended approach](./clients/index.md)), some tools
hard-code their provider endpoints. AI Bridge Proxy intercepts this traffic
transparently.

## When to use AI Bridge Proxy

Use AI Bridge Proxy when:

- An AI tool does not support customizing the base URL for API requests.
- You want to intercept traffic from tools that hard-code provider endpoints.
- You need a transparent proxy layer that doesn't require client-side changes.

For tools that support base URL overrides, use the standard
[client configuration](./clients/index.md) instead — it's simpler and more
reliable.

## How it works

AI Bridge Proxy runs as a local proxy within the workspace. It intercepts
outbound HTTPS requests to known LLM provider endpoints and redirects them
through AI Bridge, adding Coder authentication and enabling audit logging.

The proxy uses a CA certificate to perform TLS interception on requests to
allowlisted provider domains. Non-allowlisted traffic passes through untouched.

## Configuration

### Server-side settings

| Setting | Description |
|---------|-------------|
| `CODER_AI_BRIDGE_PROXY_CA_KEY` | Path to the CA private key file for AI Bridge Proxy. |
| `CODER_AI_BRIDGE_PROXY_UPSTREAM_URL` | URL of an upstream HTTP proxy to chain requests through. Format: `http://[user:pass@]host:port` |
| `CODER_AI_BRIDGE_PROXY_UPSTREAM_CA` | Path to a PEM-encoded CA certificate to trust for the upstream proxy's TLS connection. Only needed for HTTPS upstream proxies with certificates not trusted by the system. |

## Corporate HTTP proxy integration

AI Bridge Proxy works with existing corporate HTTP proxies. If your Coder
deployment routes outbound traffic through a corporate proxy to reach external
endpoints, configure the upstream proxy URL:

```bash
CODER_AI_BRIDGE_PROXY_UPSTREAM_URL=http://corporate-proxy.internal:8080
```

For HTTPS proxies with custom CA certificates:

```bash
CODER_AI_BRIDGE_PROXY_UPSTREAM_URL=https://corporate-proxy.internal:8443
CODER_AI_BRIDGE_PROXY_UPSTREAM_CA=/path/to/corporate-ca.pem
```

> **Important:** Validate that your corporate proxy allows traffic to your
> model provider endpoints before enabling AI Bridge. Ensure proxy traffic
> patterns are pre-approved by your network security team.

## Next steps

- [Client Configuration](./clients/index.md) — Configure tools that support
  base URL overrides.
- [Setup](./setup.md) — Enable AI Bridge and configure providers.
- [Monitoring](./monitoring.md) — Monitor proxied traffic.
