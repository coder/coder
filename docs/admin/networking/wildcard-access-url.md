# Wildcard Access URLs

Wildcard access URLs unlock Coder's full potential for modern development workflows. While optional for basic SSH usage, this feature becomes essential when teams need web applications, development previews, or browser-based tools. **Wildcard access URLs are essential for many development workflows in Coder** - Web IDEs (code-server, VS Code Web, JupyterLab) and some development frameworks work significantly better with subdomain-based access rather than path-based URLs.

## Why configure wildcard access URLs?

### Key benefits

- **Enables port access**: Each application gets a unique subdomain with [port support](https://coder.com/docs/user-guides/workspace-access/port-forwarding#dashboard) (e.g. `8080--main--myworkspace--john.coder.example.com`).
- **Enhanced security**: Applications run in isolated subdomains with separate browser security contexts and prevents access to the Coder API from malicious JavaScript
- **Better compatibility**: Most applications are designed to work at the root of a hostname rather than at a subpath, making subdomain access more reliable

### Applications that require subdomain access

The following tools require wildcard access URL:

- **Vite dev server**: Hot module replacement and asset serving issues with path-based routing
- **React dev server**: Similar issues with hot reloading and absolute path references
- **Next.js development server**: Asset serving and routing conflicts with path-based access
- **JupyterLab**: More complex template configuration and security risks when using path-based routing
- **RStudio**: More complex template configuration and security risks when using path-based routing

## Configuration

`CODER_WILDCARD_ACCESS_URL` is necessary for [port forwarding](port-forwarding.md#dashboard) via the dashboard or running [coder_apps](../templates/index.md) on an absolute path. Set this to a wildcard subdomain that resolves to Coder (e.g. `*.coder.example.com`).

```bash
export CODER_WILDCARD_ACCESS_URL="*.coder.example.com"
coder server
```

### TLS Certificate Setup

Wildcard access URLs require a TLS certificate that covers the wildcard domain. You have several options:

> [!TIP]
> You can use a single certificate for both the access URL and wildcard access URL. The certificate CN or SANs must match the wildcard domain, such as `*.coder.example.com`.

#### Direct TLS Configuration

Configure Coder to handle TLS directly using the wildcard certificate:

```bash
export CODER_TLS_ENABLE=true
export CODER_TLS_CERT_FILE=/path/to/wildcard.crt
export CODER_TLS_KEY_FILE=/path/to/wildcard.key
```

See [TLS & Reverse Proxy](../setup/index.md#tls--reverse-proxy) for detailed configuration options.

#### Reverse Proxy with Let's Encrypt

Use a reverse proxy to handle TLS termination with automatic certificate management:

- [NGINX with Let's Encrypt](../../tutorials/reverse-proxy-nginx.md)
- [Apache with Let's Encrypt](../../tutorials/reverse-proxy-apache.md)
- [Caddy reverse proxy](../../tutorials/reverse-proxy-caddy.md)

### DNS Setup

You'll need to configure DNS to point wildcard subdomains to your Coder server:

> [!NOTE]
> We do not recommend using a top-level-domain for Coder wildcard access
> (for example `*.workspaces`), even on private networks with split-DNS. Some
> browsers consider these "public" domains and will refuse Coder's cookies,
> which are vital to the proper operation of this feature.

```text
*.coder.example.com    A    <your-coder-server-ip>
```

Or alternatively, using a CNAME record:

```text
*.coder.example.com    CNAME    coder.example.com
```

### Workspace Proxies

If you're using [workspace proxies](workspace-proxies.md) for geo-distributed teams, each proxy requires its own wildcard access URL configuration:

```bash
# Main Coder server
export CODER_WILDCARD_ACCESS_URL="*.coder.example.com"

# Sydney workspace proxy
export CODER_WILDCARD_ACCESS_URL="*.sydney.coder.example.com"

# London workspace proxy
export CODER_WILDCARD_ACCESS_URL="*.london.coder.example.com"
```

Each proxy's wildcard domain must have corresponding DNS records:

```text
*.sydney.coder.example.com    A    <sydney-proxy-ip>
*.london.coder.example.com    A    <london-proxy-ip>
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

1. Verify the `CODER_WILDCARD_ACCESS_URL` environment variable is configured correctly:
   - Check the deployment settings in the Coder dashboard (Settings > Deployment)
   - Ensure it matches your wildcard domain (e.g., `*.coder.example.com`)
   - Restart the Coder server if you made changes to the environment variable
2. Check DNS resolution for wildcard subdomains:

   ```bash
   dig test.coder.example.com
   nslookup test.coder.example.com
   ```

3. Ensure TLS certificates cover the wildcard domain
4. Confirm template `coder_app` resources have `subdomain = true`

## See also

- [Workspace Proxies](workspace-proxies.md) - Improve performance for geo-distributed teams using wildcard URLs
