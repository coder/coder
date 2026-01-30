# Setup

AI Bridge Proxy runs inside the Coder control plane (`coderd`), requiring no separate compute to deploy or scale.
Once enabled, `coderd` runs the `aibridgeproxyd` in-memory and intercepts traffic to supported AI providers, forwarding it to AI Bridge.

**Required:**

1. AI Bridge must be enabled and configured (requires a **premium** license). See [AI Bridge Setup](../setup.md) for further information.
1. AI Bridge Proxy must be [enabled](#proxy-configuration) using the server flag.
1. A [CA certificate](#ca-certificate) must be configured for MITM interception.
1. Clients must be configured to trust the CA certificate and use the proxy.

> [!WARNING]
> AI Bridge Proxy should only be accessible within a trusted network and **must not** be directly exposed to the public internet.
> See [Security Considerations](#security-considerations) for details.

## Proxy Configuration

AI Bridge Proxy is disabled by default. To enable it, set the following configuration options:

```shell
CODER_AIBRIDGE_ENABLED=true \
CODER_AIBRIDGE_PROXY_ENABLED=true \
CODER_AIBRIDGE_PROXY_CERT_FILE=/path/to/ca.crt \
CODER_AIBRIDGE_PROXY_KEY_FILE=/path/to/ca.key \
coder server
# or via CLI flags:
coder server \
  --aibridge-enabled=true \
  --aibridge-proxy-enabled=true \
  --aibridge-proxy-cert-file=/path/to/ca.crt \
  --aibridge-proxy-key-file=/path/to/ca.key
```

Both the certificate and private key are required for AI Bridge Proxy to start.
See [CA Certificate](#ca-certificate) for how to generate and obtain these files.

The AI Bridge Proxy only intercepts and forwards traffic to AI Bridge for the supported AI provider domains:

* [Anthropic](https://www.anthropic.com/): `api.anthropic.com`
* [OpenAI](https://openai.com/): `api.openai.com`
* [GitHub Copilot](https://github.com/copilot): `api.individual.githubcopilot.com`

All other traffic is tunneled through without decryption.

For additional configuration options, see the [CLI reference](../../../reference/cli/index.md).

## Security Considerations

> [!WARNING]
> AI Bridge Proxy uses HTTP for incoming connections from AI tools.
> If exposed to untrusted networks, Coder credentials may be intercepted and internal services may become accessible to attackers.
> The AI Bridge Proxy **must not** be exposed to the public internet.

AI Bridge Proxy is an HTTP proxy, which means:

* **Credentials are sent in plain text:** In order to authenticate with Coder via AI Bridge, AI tools send the Coder session token in the proxy credentials over HTTP.
The proxy then relays these credentials to AI Bridge for authentication.
If the proxy is exposed to an untrusted network, these credentials could be intercepted.
* **Open tunnel access:** Requests to non-allowlisted domains are tunneled through the proxy without restriction.
An attacker with access to the proxy could use it to reach internal services or route traffic through the infrastructure.

These risks apply only to the connection phase between AI tools and the proxy.
Once connected:

* MITM mode: A TLS connection is established between the AI tool and the proxy (using the configured CA certificate), then traffic is forwarded securely to AI Bridge.
* Tunnel mode: A TLS connection is established directly between the AI tool and the destination, passing through the proxy without decryption.

### Deployment options

To address these risks, the recommended deployment options are:

* Internal network only: Deploy the proxy so that only AI tools within the same internal network can access it.
This is the simplest and safest approach when AI tools run inside Coder workspaces on the same network as the Coder deployment.
* TLS-terminating load balancer: Place a TLS-terminating load balancer in front of the proxy so AI tools connect to the load balancer over HTTPS.
The load balancer terminates TLS and forwards requests to the proxy over HTTP.
This protects credentials in transit, but you must still restrict access to allowed source IPs to prevent unauthorized use.

## CA Certificate

AI Bridge Proxy uses a CA (Certificate Authority) certificate to perform MITM interception of HTTPS traffic.
When AI tools connect to AI provider domains through the proxy, the proxy presents a certificate signed by this CA.
AI tools must trust this CA certificate, otherwise, the connection will fail.

### Self-signed certificate

Use a self-signed certificate when your organization doesn't have an internal CA, or when you want a dedicated CA specifically for AI Bridge Proxy.

Generate a CA certificate specifically for AI Bridge Proxy:

1) Generate a private key:

```shell
openssl genrsa -out ca.key 4096
chmod 400 ca.key
```

1) Create a self-signed CA certificate (valid for 10 years):

```shell
openssl req -new -x509 -days 3650 \
  -key ca.key \
  -out ca.crt \
  -subj "/CN=AI Bridge Proxy CA"
```

Configure AI Bridge Proxy with both files:

```shell
CODER_AIBRIDGE_PROXY_CERT_FILE=/path/to/ca.crt
CODER_AIBRIDGE_PROXY_KEY_FILE=/path/to/ca.key
```

### Organization-signed certificate

If your organization has an internal CA that clients already trust, you can have it issue an intermediate CA certificate for AI Bridge Proxy.
This simplifies deployment since AI tools that already trust your organization's root CA will automatically trust certificates signed by the intermediate.

Your organization's CA issues a certificate and private key pair for the proxy. Configure the proxy with both files:

```shell
CODER_AIBRIDGE_PROXY_CERT_FILE=/path/to/intermediate-ca.crt
CODER_AIBRIDGE_PROXY_KEY_FILE=/path/to/intermediate-ca.key
```

### Securing the private key

> [!WARNING]
> The CA private key is used to sign certificates for MITM interception.
> Store it securely and restrict access. If compromised, an attacker could intercept traffic from any client that trusts the CA certificate.

Best practices:

* Restrict file permissions so only the Coder process can read the key.
* Use a secrets manager to store the key where possible.

### Distributing the certificate

AI tools need to trust the CA certificate before connecting through the proxy.

For **self-signed certificates**, AI tools must be configured to trust the CA certificate. The certificate (without the private key) is available at:

```shell
https://<coder-url>/api/v2/aibridge/proxy/ca-cert.pem
```

For **organization-signed certificates**, if the systems where AI tools run already trust your organization's root CA, and the intermediate certificate chains correctly to that root, no additional certificate distribution is needed.
Otherwise, AI tools must be configured to trust the intermediate CA certificate from the endpoint above.

How you configure AI tools to trust the certificate depends on the tool and operating system. See Client Configuration for details.

## Upstream proxy

If your organization requires all outbound traffic to pass through a corporate proxy, you can configure AI Bridge Proxy to chain requests to an upstream proxy.

> [!NOTE]
> AI Bridge Proxy must be the first proxy in the chain.
> AI tools must be configured to connect directly to AI Bridge Proxy, which then forwards tunneled traffic to the upstream proxy.

### How it works

Tunneled requests (non-allowlisted domains) are forwarded to the upstream proxy configured via [`CODER_AIBRIDGE_PROXY_UPSTREAM`](../../../reference/cli/server.md#--aibridge-proxy-upstream).

MITM'd requests (AI provider domains) are forwarded to AI Bridge, which then communicates with AI providers.
To ensure AI Bridge also routes requests through the upstream proxy, make sure to configure the proxy settings for the Coder server process.

<!-- TODO(ssncferreira): Add diagram showing how AI Bridge Proxy integrates with upstream proxies -->

### Configuration

Configure the upstream proxy URL:

```shell
CODER_AIBRIDGE_PROXY_UPSTREAM=http://<corporate-proxy-url>:8080
```

For HTTPS upstream proxies, if the upstream proxy uses a certificate not trusted by the system, provide the CA certificate:

```shell
CODER_AIBRIDGE_PROXY_UPSTREAM=https://<corporate-proxy-url>:8080
CODER_AIBRIDGE_PROXY_UPSTREAM_CA=/path/to/corporate-ca.crt
```

If the system already trusts the upstream proxy's CA certificate, [`CODER_AIBRIDGE_PROXY_UPSTREAM_CA`](../../../reference/cli/server.md#--aibridge-proxy-upstream-ca) is not required.

<!-- TODO(ssncferreira): Add Client Configuration section -->

<!-- TODO(ssncferreira): Add Troubleshooting section -->
