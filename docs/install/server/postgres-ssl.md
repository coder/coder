---
title: Configure Coder to connect to PostgreSQL using SSL
---

<div>
  <a href="https://github.com/ericpaulsen" style="text-decoration: none; color: inherit;">
    <span style="vertical-align:middle;">Eric Paulsen</span>
  </a>
</div>
February 24, 2024

---

Your organization might require connecting to the database instance over Secure Sockets Layer (SSL).
To supply Coder with the appropriate certificates, and have it connect over SSL, follow the steps below:

## Client verification (server verifies the client)

1. Create a Kubernetes secret that holds the client certificate and its private key, if not already present:

   ```sh
   kubectl create secret generic postgres-certs -n coder --from-file=tls.crt=postgres.crt --from-file=tls.key=postgres.key
   ```

1. Define the secret volume and volumeMounts in the Helm chart:

   ```yaml
   coder:
     podSecurityContext:
       fsGroup: 1000
     volumes:
       - name: "pg-certs-mount"
         secret:
           secretName: "postgres-certs"
           defaultMode: 416
     volumeMounts:
       - name: "pg-certs-mount"
         mountPath: "/home/coder/.postgresql"
         readOnly: true
   ```

   The `fsGroup` setting gives the Coder user, which runs as user ID (UID) 1000, group access to the mounted files.
   The `defaultMode` value `416` is octal `0640`, which lets that group read the key.

1. Lastly, your PostgreSQL (PG) connection URL will look like:

   ```sh
   postgres://<user>:<password>@databasehost:<port>/<db-name>?sslmode=require&sslcert=/home/coder/.postgresql/tls.crt&sslkey=/home/coder/.postgresql/tls.key
   ```

## Server verification (client verifies the server)

1. Download the database certificate authority (CA) certificate chain, and save it as `postgres-root.crt`.

1. If you don't use client verification, create the `postgres-certs` secret with the CA file only:

   ```sh
   kubectl create secret generic postgres-certs -n coder --from-file=postgres-root.crt=postgres-root.crt
   ```

1. If you already created the secret for client verification, recreate it with all three files:

   ```sh
   kubectl create secret generic postgres-certs -n coder --from-file=tls.crt=postgres.crt --from-file=tls.key=postgres.key --from-file=postgres-root.crt=postgres-root.crt --dry-run=client -o yaml | kubectl apply -f -
   ```

1. If you don't use client verification, add the volume and volumeMounts from the client verification section to the Helm chart.

   If you use client verification, the volume already mounts the CA file, so the Helm chart doesn't change.

1. Lastly, your PG connection URL will look like:

   ```sh
   postgres://<user>:<password>@databasehost:<port>/<db-name>?sslmode=verify-full&sslrootcert=/home/coder/.postgresql/postgres-root.crt
   ```

If your database also requires a client certificate, keep the `sslcert` and `sslkey` parameters from the client verification section.

> [!NOTE]
> If you set `sslrootcert` with `sslmode=require` and the file is missing, the driver skips server verification instead of failing.
> Use `sslmode=verify-full` so that a missing CA file stops the connection.

More information on connecting to PostgreSQL databases using certificates can be found in the [PostgreSQL documentation](https://www.postgresql.org/docs/current/libpq-ssl.html#LIBPQ-SSL-CLIENTCERT).
