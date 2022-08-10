# Configure

This article documents the Coder server's primary configuration variables. For a full list
of the options, run `coder server --help` on the host.

Once you've [installed](../install.md) Coder, you can configure the server by setting the following
variables in `/etc/coder.d/coder.env`:

```sh
# String. Specifies the external URL (HTTP/S) to access Coder.
CODER_ACCESS_URL=https://coder.example.com

# String. Address to serve the API and dashboard.
CODER_ADDRESS=127.0.0.1:3000

# String. The URL of a PostgreSQL database to connect to. If empty, PostgreSQL binaries
# will be downloaded from Maven (https://repo1.maven.org/maven2) and store all
# data in the config root. Access the built-in database with "coder server postgres-builtin-url".
CODER_PG_CONNECTION_URL=""

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

## Run Coder

Now, run Coder as a system service on the host:

```sh
# Use systemd to start Coder now and on reboot
sudo systemctl enable --now coder
# View the logs to ensure a successful start
journalctl -u coder.service -b
```

## Up Next

- [Get started using Coder](../quickstart.md).
- [Learn how to upgrade Coder](./upgrade.md).
