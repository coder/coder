Coder server's primary configuration is done via environment variables. For a
full list of the options, run `coder server --help` or see our
[CLI documentation](../cli/server.md).

## Access URL

`CODER_ACCESS_URL` is required if you are not using the tunnel. Set this to the
external URL that users and workspaces use to connect to Coder (e.g.
<https://coder.example.com>). This should not be localhost.

> Access URL should be an external IP address or domain with DNS records
> pointing to Coder.

### Tunnel

If an access URL is not specified, Coder will create a publicly accessible URL
to reverse proxy your deployment for simple setup.

## Address

You can change which port(s) Coder listens on.

```shell
# Listen on port 80
export CODER_HTTP_ADDRESS=0.0.0.0:80

# Enable TLS and listen on port 443)
export CODER_TLS_ENABLE=true
export CODER_TLS_ADDRESS=0.0.0.0:443

## Redirect from HTTP to HTTPS
export CODER_REDIRECT_TO_ACCESS_URL=true

# Start the Coder server
coder server
```

## Wildcard access URL

`CODER_WILDCARD_ACCESS_URL` is necessary for
[port forwarding](../networking/port-forwarding.md#dashboard) via the dashboard
or running [coder_apps](../templates/index.md#coder-apps) on an absolute path.
Set this to a wildcard subdomain that resolves to Coder (e.g.
`*.coder.example.com`).

If you are providing TLS certificates directly to the Coder server, either

1. Use a single certificate and key for both the root and wildcard domains.
2. Configure multiple certificates and keys via
   [`coder.tls.secretNames`](https://github.com/coder/coder/blob/main/helm/coder/values.yaml)
   in the Helm Chart, or [`--tls-cert-file`](../cli/server.md#--tls-cert-file)
   and [`--tls-key-file`](../cli/server.md#--tls-key-file) command line options
   (these both take a comma separated list of files; list certificates and their
   respective keys in the same order).

## TLS & Reverse Proxy

The Coder server can directly use TLS certificates with `CODER_TLS_ENABLE` and
accompanying configuration flags. However, Coder can also run behind a
reverse-proxy to terminate TLS certificates from LetsEncrypt, for example.

- [Apache](https://github.com/coder/coder/tree/main/examples/web-server/apache)
- [Caddy](https://github.com/coder/coder/tree/main/examples/web-server/caddy)
- [NGINX](https://github.com/coder/coder/tree/main/examples/web-server/nginx)

### Kubernetes TLS configuration

Below are the steps to configure Coder to terminate TLS when running on
Kubernetes. You must have the certificate `.key` and `.crt` files in your
working directory prior to step 1.

1. Create the TLS secret in your Kubernetes cluster

```shell
kubectl create secret tls coder-tls -n <coder-namespace> --key="tls.key" --cert="tls.crt"
```

> You can use a single certificate for the both the access URL and wildcard
> access URL. The certificate CN must match the wildcard domain, such as
> `*.example.coder.com`.

1. Reference the TLS secret in your Coder Helm chart values

```yaml
coder:
  tls:
    secretName:
      - coder-tls

  # Alternatively, if you use an Ingress controller to terminate TLS,
  # set the following values:
  ingress:
    enable: true
    secretName: coder-tls
    wildcardSecretName: coder-tls
```

## PostgreSQL Database

Coder uses a PostgreSQL database to store users, workspace metadata, and other
deployment information. Use `CODER_PG_CONNECTION_URL` to set the database that
Coder connects to. If unset, PostgreSQL binaries will be downloaded from Maven
(<https://repo1.maven.org/maven2>) and store all data in the config root.

> Postgres 13 is the minimum supported version.

If you are using the built-in PostgreSQL deployment and need to use `psql` (aka
the PostgreSQL interactive terminal), output the connection URL with the
following command:

```console
coder server postgres-builtin-url
psql "postgres://coder@localhost:49627/coder?sslmode=disable&password=feU...yI1"
```

### Migrating from the built-in database to an external database

To migrate from the built-in database to an external database, follow these
steps:

1. Stop your Coder deployment.
2. Run `coder server postgres-builtin-serve` in a background terminal.
3. Run `coder server postgres-builtin-url` and copy its output command.
4. Run `pg_dump <built-in-connection-string> > coder.sql` to dump the internal
   database to a file.
5. Restore that content to an external database with
   `psql <external-connection-string> < coder.sql`.
6. Start your Coder deployment with
   `CODER_PG_CONNECTION_URL=<external-connection-string>`.

## System packages

If you've installed Coder via a [system package](../install/index.md) Coder, you
can configure the server by setting the following variables in
`/etc/coder.d/coder.env`:

```env
# String. Specifies the external URL (HTTP/S) to access Coder.
CODER_ACCESS_URL=https://coder.example.com

# String. Address to serve the API and dashboard.
CODER_HTTP_ADDRESS=0.0.0.0:3000

# String. The URL of a PostgreSQL database to connect to. If empty, PostgreSQL binaries
# will be downloaded from Maven (https://repo1.maven.org/maven2) and store all
# data in the config root. Access the built-in database with "coder server postgres-builtin-url".
CODER_PG_CONNECTION_URL=

# Boolean. Specifies if TLS will be enabled.
CODER_TLS_ENABLE=

# If CODER_TLS_ENABLE=true, also set:
CODER_TLS_ADDRESS=0.0.0.0:3443

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

```shell
# Use systemd to start Coder now and on reboot
sudo systemctl enable --now coder

# View the logs to ensure a successful start
journalctl -u coder.service -b
```

To restart Coder after applying system changes:

```shell
sudo systemctl restart coder
```

## Configuring Coder behind a proxy

To configure Coder behind a corporate proxy, set the environment variables
`HTTP_PROXY` and `HTTPS_PROXY`. Be sure to restart the server. Lowercase values
(e.g. `http_proxy`) are also respected in this case.

## Up Next

- [Learn how to upgrade Coder](./upgrade.md).
