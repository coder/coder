# Wildcard Access URLs

> [!IMPORTANT]
> **Wildcard access URL is essential for many development workflows in Coder.** Web IDEs (code-server, VS Code Web, JupyterLab) and some development frameworks work significantly better with subdomain-based access rather than path-based URLs.

Wildcard access URLs unlock Coder's full potential for modern development workflows. While optional for basic SSH usage, this feature becomes essential when teams need web applications, development previews, or browser-based tools.

## Why configure wildcard access URLs?

### Key benefits

- **Eliminates port conflicts**: Each application gets a unique subdomain (e.g., `8080--main--myworkspace--john.coder.example.com`)
- **Enhanced security**: Applications run in isolated subdomains with separate browser security contexts
- **Better compatibility**: Web-based IDEs, mobile devices, and third-party integrations work reliably with standard HTTPS URLs

### When wildcard access URL is required

Wildcard access URL enables subdomain-based workspace applications, which is required for:

- **Web IDEs**: code-server, VS Code Web, JupyterLab, RStudio work better with dedicated subdomains
- **Modern development frameworks**: Vite, React dev server, Next.js, and similar tools expect to control the entire domain for features like hot module replacement and asset serving
- **Development servers with preview URLs**: Applications that generate preview URLs or use absolute paths
- **Applications that don't support path-based routing**: Some tools like KasmVNC cannot function with path-based access
- **Secure development environment isolation**: Each development application runs on its own subdomain

## Configuration

`CODER_WILDCARD_ACCESS_URL` is necessary for [port forwarding](port-forwarding.md#dashboard) via the dashboard or running [coder_apps](../templates/index.md) on an absolute path. Set this to a wildcard subdomain that resolves to Coder (e.g. `*.coder.example.com`).

```bash
export CODER_WILDCARD_ACCESS_URL="*.coder.example.com"
coder server
```

### DNS Setup

You'll need to configure DNS to point wildcard subdomains to your Coder server:

```text
*.coder.example.com    A    <your-coder-server-ip>
```

## Template Configuration

In your Coder templates, enable subdomain applications using the `subdomain` parameter:

```hcl
resource "coder_app" "code-server" {
  agent_id     = coder_agent.main.id
  slug         = "code-server"
  display_name = "VS Code"
  url          = "http://localhost:8080"
  icon         = "/icon/code.svg"
  subdomain    = true
  share        = "owner"
}
```

## Troubleshooting

### Applications not accessible

If workspace applications are not working:

1. Verify the `CODER_WILDCARD_ACCESS_URL` environment variable is configured correctly
2. Check DNS resolution for wildcard subdomains
3. Ensure TLS certificates cover the wildcard domain
4. Confirm template `coder_app` resources have `subdomain = true`

## See also

- [Port Forwarding](port-forwarding.md) - Access workspace applications via dashboard when wildcard URL is not configured
- [Workspace Proxies](workspace-proxies.md) - Improve performance for geo-distributed teams using wildcard URLs
