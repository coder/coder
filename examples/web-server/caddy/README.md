# Caddy

This is an example configuration of how to use Coder with [caddy](https://caddyserver.com/docs). To use Caddy to generate TLS certificates, you'll need a domain name that resolves to your Caddy server.

## Getting started

### With docker-compose

1. [Install Docker](https://docs.docker.com/engine/install/) and [Docker Compose](https://docs.docker.com/compose/install/)

1. Start with our example configuration

   ```console
   # Create a project folder
   cd $HOME
   mkdir coder-with-caddy
   cd coder-with-caddy

   # Clone coder/coder and copy the Caddy example
   git clone https://github.com/coder/coder /tmp/coder
   mv /tmp/coder/examples/web-server/caddy $(pwd)
   ```

1. Modify the [Caddyfile](./Caddyfile) and change the following values:

   - `localhost:3000`: Change to `coder:7080` (Coder container on Docker network)
   - `email@example.com`: Email to request certificates from LetsEncrypt/ZeroSSL (does not have to be Coder admin email)
   - `coder.example.com`: Domain name you're using for Coder.
   - `*.coder.example.com`: Domain name for wildcard apps, commonly used for [dashboard port forwarding](https://coder.com/docs/coder-oss/latest/networking/port-forwarding#dashboard). This is optional and can be removed.

1. Start Coder. Set `CODER_ACCESS_URL` and `CODER_WILDCARD_ACCESS_URL` to the domain you're using in your Caddyfile.

   ```console
   export CODER_ACCESS_URL=https://coder.example.com
   export CODER_WILDCARD_ACCESS_URL=*.coder.example.com
   docker compose up -d # Run on startup
   ```

### Standalone

1. If you haven't already, [install Coder](https://coder.com/docs/coder-oss/latest/install)

2. Install [Caddy Server](https://caddyserver.com/docs/install)

3. Copy our sample [Caddyfile](./Caddyfile) and change the following values:

   > If you're installed Caddy as a system package, update the default Caddyfile with `vim /etc/caddy/Caddyfile`

   - `email@example.com`: Email to request certificates from LetsEncrypt/ZeroSSL (does not have to be Coder admin email)
   - `coder.example.com`: Domain name you're using for Coder.
   - `*.coder.example.com`: Domain name for wildcard apps, commonly used for [dashboard port forwarding](https://coder.com/docs/coder-oss/latest/networking/port-forwarding#dashboard). This is optional and can be removed.
   - `localhost:3000`: Address Coder is running on. Modify this if you changed `CODER_HTTP_ADDRESS` in the Coder configuration.

4. [Configure Coder](https://coder.com/docs/coder-oss/latest/admin/configure) and change the following values:

   - `CODER_ACCESS_URL`: root domain (e.g. `https://coder.example.com`)
   - `CODER_WILDCARD_ACCESS_URL`: wildcard domain (e.g. `*.example.com`).

5. Start the Caddy server:

   If you're [keeping Caddy running](https://caddyserver.com/docs/running) via a system service:

   ```console
   sudo systemctl restart caddy
   ```

   Or run a standalone server:

   ```console
   caddy run
   ```

6. Optionally, use [ufw](https://wiki.ubuntu.com/UncomplicatedFirewall) or another firewall to disable external traffic outside of Caddy.

   ```console
   # Check status of UncomplicatedFirewall
   sudo ufw status

   # Allow SSH
   sudo ufw allow 22

   # Allow HTTP, HTTPS (Caddy)
   sudo ufw allow 80
   sudo ufw allow 443

   # Deny direct access to Coder server
   sudo ufw deny 3000

   # Enable UncomplicatedFirewall
   sudo ufw enable
   ```

7. Navigate to your Coder URL! A TLS certificate should be auto-generated on your first visit.

## Generating wildcard certificates

By default, this configuration uses Caddy's [on-demand TLS](https://caddyserver.com/docs/caddyfile/options#on-demand-tls) to generate a certificate for each subdomain (e.g. `app1.coder.example.com`, `app2.coder.example.com`). When users visit new subdomains, such as accessing [ports on a workspace](../../../docs/networking/port-forwarding.md), the request will take an additional 5-30 seconds since a new certificate is being generated.

For production deployments, we recommend configuring Caddy to generate a wildcard certificate, which requires an explicit DNS challenge and additional Caddy modules.

1. Install a custom Caddy build that includes the [caddy-dns](https://github.com/caddy-dns) module for your DNS provider (e.g. CloudFlare, Route53).

   - Docker: [Build an custom Caddy image](https://github.com/docker-library/docs/tree/master/caddy#adding-custom-caddy-modules) with the module for your DNS provider. Be sure to reference the new image in the `docker-compose.yaml`.

   - Standalone: [Download a custom Caddy build](https://caddyserver.com/download) with the module for your DNS provider. If you're using Debian/Ubuntu, you [can configure the Caddy package](https://caddyserver.com/docs/build#package-support-files-for-custom-builds-for-debianubunturaspbian) to use the new build.

2. Edit your `Caddyfile` and add the necessary credentials/API tokens to solve the DNS challenge for wildcard certificates.

   For example, for AWS Route53:

   ```diff
   tls {
   -  on_demand
      issuer acme {
          email email@example.com
      }

   +  dns route53 {
   +     max_retries 10
   +     aws_profile "real-profile"
   +     access_key_id "AKI..."
   +     secret_access_key "wJa..."
   +     token "TOKEN..."
   +     region "us-east-1"
   +  }
   }
   ```

   > Configuration reference from [caddy-dns/route53](https://github.com/caddy-dns/route53).

   And for CloudFlare:

   Generate a [token](https://dash.cloudflare.com/profile/api-tokens) with the following permissions:

   - Zone:Zone:Edit

   ```diff
   tls {
   -  on_demand
     issuer acme {
         email email@example.com
     }

   +  dns cloudflare CLOUDFLARE_API_TOKEN
   }
   ```

   > Configuration reference from [caddy-dns/cloudflare](https://github.com/caddy-dns/cloudflare).
