## Requirements

Before proceeding, please ensure that you have a Kubernetes cluster running K8s
1.19+ and have Helm 3.5+ installed.

You'll also want to install the
[latest version of Coder](https://github.com/coder/coder/releases/latest)
locally in order to log in and manage templates.

## Install Coder with Helm

1. Create a namespace for Coder, such as `coder`:

   ```console
   kubectl create namespace coder
   ```

1. Create a PostgreSQL deployment. Coder does not manage a database server for
   you.

   If you're in a public cloud such as
   [Google Cloud](https://cloud.google.com/sql/docs/postgres/),
   [AWS](https://aws.amazon.com/rds/postgresql/),
   [Azure](https://docs.microsoft.com/en-us/azure/postgresql/), or
   [DigitalOcean](https://www.digitalocean.com/products/managed-databases-postgresql),
   you can use the managed PostgreSQL offerings they provide. Make sure that the
   PostgreSQL service is running and accessible from your cluster. It should be
   in the same network, same project, etc.

   You can install Postgres manually on your cluster using the
   [Bitnami PostgreSQL Helm chart](https://github.com/bitnami/charts/tree/master/bitnami/postgresql#readme).
   There are some
   [helpful guides](https://phoenixnap.com/kb/postgresql-kubernetes) on the
   internet that explain sensible configurations for this chart. Example:

   ```console
   # Install PostgreSQL
   helm repo add bitnami https://charts.bitnami.com/bitnami
   helm install coder-db bitnami/postgresql \
       --namespace coder \
       --set auth.username=coder \
       --set auth.password=coder \
       --set auth.database=coder \
       --set persistence.size=10Gi
   ```

   The cluster-internal DB URL for the above database is:

   ```shell
   postgres://coder:coder@coder-db-postgresql.coder.svc.cluster.local:5432/coder?sslmode=disable
   ```

   > Ensure you set up periodic backups so you don't lose data.

   You can use [Postgres operator](https://github.com/zalando/postgres-operator)
   to manage PostgreSQL deployments on your Kubernetes cluster.

1. Create a secret with the database URL:

   ```shell
   # Uses Bitnami PostgreSQL example. If you have another database,
   # change to the proper URL.
   kubectl create secret generic coder-db-url -n coder \
      --from-literal=url="postgres://coder:coder@coder-db-postgresql.coder.svc.cluster.local:5432/coder?sslmode=disable"
   ```

1. Add the Coder Helm repo:

   ```shell
   helm repo add coder-v2 https://helm.coder.com/v2
   ```

1. Create a `values.yaml` with the configuration settings you'd like for your
   deployment. For example:

   ```yaml
   coder:
     # You can specify any environment variables you'd like to pass to Coder
     # here. Coder consumes environment variables listed in
     # `coder server --help`, and these environment variables are also passed
     # to the workspace provisioner (so you can consume them in your Terraform
     # templates for auth keys etc.).
     #
     # Please keep in mind that you should not set `CODER_HTTP_ADDRESS`,
     # `CODER_TLS_ENABLE`, `CODER_TLS_CERT_FILE` or `CODER_TLS_KEY_FILE` as
     # they are already set by the Helm chart and will cause conflicts.
     env:
       - name: CODER_PG_CONNECTION_URL
         valueFrom:
           secretKeyRef:
             # You'll need to create a secret called coder-db-url with your
             # Postgres connection URL like:
             # postgres://coder:password@postgres:5432/coder?sslmode=disable
             name: coder-db-url
             key: url

       # (Optional) For production deployments the access URL should be set.
       # If you're just trying Coder, access the dashboard via the service IP.
       - name: CODER_ACCESS_URL
         value: "https://coder.example.com"

     #tls:
     #  secretNames:
     #    - my-tls-secret-name
   ```

   > You can view our
   > [Helm README](https://github.com/coder/coder/blob/main/helm#readme) for
   > details on the values that are available, or you can view the
   > [values.yaml](https://github.com/coder/coder/blob/main/helm/coder/values.yaml)
   > file directly.

1. Run the following command to install the chart in your cluster.

   ```shell
   helm install coder coder-v2/coder \
       --namespace coder \
       --values values.yaml
   ```

   You can watch Coder start up by running `kubectl get pods -n coder`. Once
   Coder has started, the `coder-*` pods should enter the `Running` state.

1. Log in to Coder

   Use `kubectl get svc -n coder` to get the IP address of the LoadBalancer.
   Visit this in the browser to set up your first account.

   If you do not have a domain, you should set `CODER_ACCESS_URL` to this URL in
   the Helm chart and upgrade Coder (see below). This allows workspaces to
   connect to the proper Coder URL.

## Upgrading Coder via Helm

To upgrade Coder in the future or change values, you can run the following
command:

```shell
helm repo update
helm upgrade coder coder-v2/coder \
  --namespace coder \
  -f values.yaml
```

## Load balancing considerations

### AWS

If you are deploying Coder on AWS EKS and service is set to `LoadBalancer`, AWS
will default to the Classic load balancer. The load balancer external IP will be
stuck in a pending status unless sessionAffinity is set to None.

```yaml
coder:
  service:
    type: LoadBalancer
    sessionAffinity: None
```

AWS recommends a Network load balancer in lieu of the Classic load balancer. Use
the following `values.yaml` settings to request a Network load balancer:

```yaml
coder:
  service:
    externalTrafficPolicy: Local
    sessionAffinity: None
    annotations: { service.beta.kubernetes.io/aws-load-balancer-type: "nlb" }
```

By default, Coder will set the `externalTrafficPolicy` to `Cluster` which will
mask client IP addresses in the Audit log. To preserve the source IP, you can
either set this value to `Local`, or pass through the client IP via the
X-Forwarded-For header. To configure the latter, set the following environment
variables:

```yaml
coder:
  env:
    - name: CODER_PROXY_TRUSTED_HEADERS
      value: X-Forwarded-For
    - name: CODER_PROXY_TRUSTED_ORIGINS
      value: 10.0.0.1/8 # this will be the CIDR range of your Load Balancer IP address
```

### Azure

In certain enterprise environments, the
[Azure Application Gateway](https://learn.microsoft.com/en-us/azure/application-gateway/ingress-controller-overview)
was needed. The Application Gateway supports:

- Websocket traffic (required for workspace connections)
- TLS termination

## PostgreSQL Certificates

Your organization may require connecting to the database instance over SSL. To
supply Coder with the appropriate certificates, and have it connect over SSL,
follow the steps below:

### Client verification (server verifies the client)

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

### Server verification (client verifies the server)

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

> More information on connecting to PostgreSQL databases using certificates can
> be found
> [here](https://www.postgresql.org/docs/current/libpq-ssl.html#LIBPQ-SSL-CLIENTCERT).

## Troubleshooting

You can view Coder's logs by getting the pod name from `kubectl get pods` and
then running `kubectl logs <pod name>`. You can also view these logs in your
Cloud's log management system if you are using managed Kubernetes.

### Kubernetes-based workspace is stuck in "Connecting..."

Ensure you have an externally-reachable `CODER_ACCESS_URL` set in your helm
chart. If you do not have a domain set up, this should be the IP address of
Coder's LoadBalancer (`kubectl get svc -n coder`).

See [troubleshooting templates](../templates/index.md#troubleshooting-templates)
for more steps.

## Next steps

- [Configuring Coder](../admin/configure.md)
- [Templates](../templates/index.md)
