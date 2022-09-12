Coder server's primary configuration is done via environment variables. For a full list
of the options, run `coder server --help` on the host.

## Tunnel

For proof-of-concept deployments, you can set `CODER_TUNNEL=true` to run Coder on a unique `*.try.coder.app` URL.
This is a quick way to allow users and workspaces outside your LAN to connect to Coder.

## Access URL

`CODER_ACCESS_URL` is required if you are not using the tunnel. Set this to the external URL
that users and workspaces use to connect to Coder (e.g. https://coder.example.com). This
should not be localhost.

> Access URL should be a external IP address or domain with DNS records pointing to Coder.

## PostgreSQL Database

Coder uses a PostgreSQL database to store users, workspace metadata, and other deployment information.
Use `CODER_PG_CONNECTION_URL` to set the database that Coder connects to. If unset, PostgreSQL binaries will be
downloaded from Maven (https://repo1.maven.org/maven2) and store all data in the config root.

## System packages

If you've installed Coder via a [system package](../install/packages.md) Coder, you can
configure the server by setting the following variables in `/etc/coder.d/coder.env`:

```sh
# String. Specifies the external URL (HTTP/S) to access Coder.
CODER_ACCESS_URL=https://coder.example.com

# String. Address to serve the API and dashboard.
CODER_ADDRESS=127.0.0.1:3000

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

```sh
# Use systemd to start Coder now and on reboot
sudo systemctl enable --now coder

# View the logs to ensure a successful start
journalctl -u coder.service -b
```

To restart Coder after applying system changes:

```sh
sudo systemctl restart Coder
```

## Up Next

- [Get started using Coder](../quickstart.md).
- [Learn how to upgrade Coder](./upgrade.md).
