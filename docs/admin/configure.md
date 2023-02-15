Coder server's primary configuration is done via environment variables. For a full list
of the options, run `coder server --help` on the host.

## Access URL

`CODER_ACCESS_URL` is required if you are not using the tunnel. Set this to the external URL
that users and workspaces use to connect to Coder (e.g. <https://coder.example.com>). This
should not be localhost.

> Access URL should be a external IP address or domain with DNS records pointing to Coder.

### Tunnel

If an access URL is not specified, Coder will create
a publicly accessible URL to reverse proxy your deployment for simple setup.

## Address

You can change which port(s) Coder listens on.

```sh
# Listen on port 80
export CODER_HTTP_ADDRESS=0.0.0.0:80

# Enable TLS and listen on port 443)
export CODER_TLS_ENABLE=true
export CODER_TLS_ADDRESS=0.0.0.0:443

## Redirect from HTTP to HTTPS
export CODER_TLS_REDIRECT_HTTP=true

# Start the Coder server
coder server
```

## Wildcard access URL

`CODER_WILDCARD_ACCESS_URL` is necessary for [port forwarding](../networking/port-forwarding.md#dashboard)
via the dashboard or running [coder_apps](../templates.md#coder-apps) on an absolute path. Set this to a wildcard
subdomain that resolves to Coder (e.g. `*.coder.example.com`).

> If you are providing TLS certificates directly to the Coder server, you must use a single certificate for the
> root and wildcard domains. Multi-certificate support [is planned](https://github.com/coder/coder/pull/4150).

## TLS & Reverse Proxy

The Coder server can directly use TLS certificates with `CODER_TLS_ENABLE` and accompanying configuration flags. However, Coder can also run behind a reverse-proxy to terminate TLS certificates from LetsEncrypt, for example.

- Apache: [Run Coder with Apache and LetsEncrypt](https://github.com/coder/coder/tree/main/examples/web-server/apache)
- Caddy: [Run Coder with Caddy and LetsEncrypt](https://github.com/coder/coder/tree/main/examples/web-server/caddy)
- NGINX: [Run Coder with Nginx and LetsEncrypt](https://github.com/coder/coder/tree/main/examples/web-server/nginx)

## PostgreSQL Database

Coder uses a PostgreSQL database to store users, workspace metadata, and other deployment information.
Use `CODER_PG_CONNECTION_URL` to set the database that Coder connects to. If unset, PostgreSQL binaries will be
downloaded from Maven (<https://repo1.maven.org/maven2>) and store all data in the config root.

> Postgres 13 is the minimum supported version.

If you are using the built-in PostgreSQL deployment and need to use `psql` (aka
the PostgreSQL interactive terminal), output the connection URL with the following command:

```console
coder server postgres-builtin-url
psql "postgres://coder@localhost:49627/coder?sslmode=disable&password=feU...yI1"
```

## System packages

If you've installed Coder via a [system package](../install/packages.md) Coder, you can
configure the server by setting the following variables in `/etc/coder.d/coder.env`:

```console
# String. Specifies the external URL (HTTP/S) to access Coder.
CODER_ACCESS_URL=https://coder.example.com

# String. Address to serve the API and dashboard.
CODER_HTTP_ADDRESS=127.0.0.1:3000

# String. The URL of a PostgreSQL database to connect to. If empty, PostgreSQL binaries
# will be downloaded from Maven (https://repo1.maven.org/maven2) and store all
# data in the config root. Access the built-in database with "coder server postgres-builtin-url".
CODER_PG_CONNECTION_URL=

# Boolean. Specifies if TLS will be enabled.
CODER_TLS_ENABLE=

# String. Specifies the path to the certificate for TLS. It requires a PEM-encoded file.
# To configure the listener to use a CA certificate, concatenate the primary
# certificate and the CA certificate together. The primary certificate should
# appear first in the combined file.
CODER_TLS_CERT_FILE=

# String. Specifies the path to the private key for the certificate. It requires a
# PEM-encoded file.
CODER_TLS_KEY_FILE=
```

To run Coder as a system service on the host:

```console
# Use systemd to start Coder now and on reboot
sudo systemctl enable --now coder

# View the logs to ensure a successful start
journalctl -u coder.service -b
```

To restart Coder after applying system changes:

```console
sudo systemctl restart coder
```

## Configuring Coder behind a proxy

To configure Coder behind a corporate proxy, set the environment variables `HTTP_PROXY` and
`HTTPS_PROXY`. Be sure to restart the server. Lowercase values (e.g. `http_proxy`) are also
respected in this case.

## Up Next

- [Get started using Coder](../quickstart.md).
- [Learn how to upgrade Coder](./upgrade.md).
