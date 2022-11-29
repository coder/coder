## Requirements

Before proceeding, please ensure that you have a Kubernetes cluster running K8s 1.19+ and have Helm 3.5+ installed.

You'll also want to install the [latest version of Coder](https://github.com/coder/coder/releases) locally in order
to log in and manage templates.

## Install Coder with Helm

> **Warning**: Helm support is new and not yet complete. There may be changes
> to the Helm chart between releases which require manual values updates. Please
> file an issue if you run into any issues.

1. Create a namespace for Coder, such as `coder`:

   ```console
   $ kubectl create namespace coder
   ```

1. Create a PostgreSQL deployment. Coder does not manage a database server for
   you.

   If you're in a public cloud such as
   [Google Cloud](https://cloud.google.com/sql/docs/postgres/),
   [AWS](https://aws.amazon.com/rds/postgresql/),
   [Azure](https://docs.microsoft.com/en-us/azure/postgresql/), or
   [DigitalOcean](https://www.digitalocean.com/products/managed-databases-postgresql),
   you can use the managed PostgreSQL offerings they provide. Make sure that
   the PostgreSQL service is running and accessible from your cluster. It
   should be in the same network, same project, etc.

   You can install Postgres manually on your cluster using the
   [Bitnami PostgreSQL Helm chart](https://github.com/bitnami/charts/tree/master/bitnami/postgresql#readme). There are some
   [helpful guides](https://phoenixnap.com/kb/postgresql-kubernetes) on the
   internet that explain sensible configurations for this chart. Example:

   ```sh
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

   ```console
   postgres://coder:coder@coder-db-postgresql.coder.svc.cluster.local:5432/coder?sslmode=disable
   ```

   > Ensure you set up periodic backups so you don't lose data.

   You can use
   [Postgres operator](https://github.com/zalando/postgres-operator) to
   manage PostgreSQL deployments on your Kubernetes cluster.

1. Add the Coder Helm repo:

   ```console
   helm repo add coder-v2 https://helm.coder.com/v2
   ```

1. Create a secret with the database URL:

   ```sh
   # Uses Bitnami PostgreSQL example. If you have another database,
   # change to the proper URL.
   kubectl create secret generic coder-db-url -n coder \
      --from-literal=url="postgres://coder:coder@coder-db-postgresql.coder.svc.cluster.local:5432/coder?sslmode=disable"
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
     # Please keep in mind that you should not set `CODER_ADDRESS`,
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

       # This env variable controls whether or not to auto-import the
       # "kubernetes" template on first startup. This will not work unless
       # coder.serviceAccount.workspacePerms is true.
       - name: CODER_AUTO_IMPORT_TEMPLATES
         value: "kubernetes"

     #tls:
     #  secretNames:
     #    - my-tls-secret-name
   ```

   > You can view our
   > [Helm README](https://github.com/coder/coder/blob/main/helm#readme) for
   > details on the values that are available, or you can view the
   > [values.yaml](https://github.com/coder/coder/blob/main/helm/values.yaml)
   > file directly.

1. Run the following command to install the chart in your cluster.

   ```sh
   helm install coder coder-v2/coder \
       --namespace coder \
       --values values.yaml
   ```

   You can watch Coder start up by running `kubectl get pods -n coder`. Once Coder has
   started, the `coder-*` pods should enter the `Running` state.

1. Log in to Coder

   Use `kubectl get svc -n coder` to get the IP address of the
   LoadBalancer. Visit this in the browser to set up your first account.

   If you do not have a domain, you should set `CODER_ACCESS_URL`
   to this URL in the Helm chart and upgrade Coder (see below).
   This allows workspaces to connect to the proper Coder URL.

## Upgrading Coder via Helm

To upgrade Coder in the future or change values,
you can run the following command:

```sh
helm repo update
helm upgrade coder coder-v2/coder \
  --namespace coder \
  -f values.yaml
```

## Troubleshooting

You can view Coder's logs by getting the pod name from `kubectl get pods` and then running `kubectl logs <pod name>`. You can also
view these logs in your
Cloud's log management system if you are using managed Kubernetes.

### Kubernetes-based workspace is stuck in "Connecting..."

Ensure you have an externally-reachable `CODER_ACCESS_URL` set in your helm chart. If you do not have a domain set up,
this should be the IP address of Coder's LoadBalancer (`kubectl get svc -n coder`).

See [troubleshooting templates](../templates.md#creating-and-troubleshooting-templates) for more steps.

## Next steps

- [Quickstart](../quickstart.md)
- [Configuring Coder](../admin/configure.md)
- [Templates](../templates.md)
