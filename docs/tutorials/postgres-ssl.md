# Configure Coder to connect to PostgreSQL using SSL

<div>
  <a href="https://github.com/ericpaulsen" style="text-decoration: none; color: inherit;">
    <span style="vertical-align:middle;">Eric Paulsen</span>
    <img src="https://github.com/ericpaulsen.png" alt="ericpaulsen" width="24px" height="24px" style="vertical-align:middle; margin: 0px;"/>
  </a>
</div>
February 24, 2024

---

Your organization may require connecting to the database instance over SSL. To
supply Coder with the appropriate certificates, and have it connect over SSL,
follow the steps below:

## Client verification (server verifies the client)

1. Create the certificate as a secret in your Kubernetes cluster, if not already
   present:

```shell
kubectl create secret tls postgres-certs -n coder --key="postgres.key" --cert="postgres.crt"
```

1. Define the secret volume and volumeMounts in the Helm chart:

```yaml
coder:
  volumes:
    - name: "pg-certs-mount"
      secret:
        secretName: "postgres-certs"
  volumeMounts:
    - name: "pg-certs-mount"
      mountPath: "$HOME/.postgresql"
      readOnly: true
```

1. Lastly, your PG connection URL will look like:

```shell
postgres://<user>:<password>@databasehost:<port>/<db-name>?sslmode=require&sslcert="$HOME/.postgresql/postgres.crt&sslkey=$HOME/.postgresql/postgres.key"
```

## Server verification (client verifies the server)

1. Download the CA certificate chain for your database instance, and create it
   as a secret in your Kubernetes cluster, if not already present:

```shell
kubectl create secret tls postgres-certs -n coder --key="postgres-root.key" --cert="postgres-root.crt"
```

1. Define the secret volume and volumeMounts in the Helm chart:

```yaml
coder:
  volumes:
    - name: "pg-certs-mount"
      secret:
        secretName: "postgres-certs"
  volumeMounts:
    - name: "pg-certs-mount"
      mountPath: "$HOME/.postgresql/postgres-root.crt"
      readOnly: true
```

1. Lastly, your PG connection URL will look like:

```shell
postgres://<user>:<password>@databasehost:<port>/<db-name>?sslmode=verify-full&sslrootcert="/home/coder/.postgresql/postgres-root.crt"
```

More information on connecting to PostgreSQL databases using certificates can
be found in the [PostgreSQL documentation](https://www.postgresql.org/docs/current/libpq-ssl.html#LIBPQ-SSL-CLIENTCERT).
