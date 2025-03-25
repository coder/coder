# Deploy Coder on Rancher

You can deploy Coder on Rancher as a
[Workload](https://ranchermanager.docs.rancher.com/getting-started/quick-start-guides/deploy-workloads/workload-ingress).

## Requirements

- [SUSE Rancher Manager](https://ranchermanager.docs.rancher.com/getting-started/installation-and-upgrade/install-upgrade-on-a-kubernetes-cluster) running Kubernetes (K8s) 1.19+ with [SUSE Rancher Prime distribution](https://documentation.suse.com/cloudnative/rancher-manager/latest/en/integrations/kubernetes-distributions.html) (Rancher Manager 2.10+)
- Helm 3.5+ installed
- Workload Kubernetes cluster for Coder

## Overview

Installing Coder on Rancher involves four key steps:

1. Create a namespace for Coder
1. Set up PostgreSQL
1. Create a database connection secret
1. Install the Coder application via Rancher UI

## Create a namespace

Create a namespace for the Coder control plane. In this tutorial, we call it `coder`:

```shell
kubectl create namespace coder
```

## Set up PostgreSQL

Coder requires a PostgreSQL database to store deployment data.
We recommend that you use a managed PostgreSQL service, but you can use an in-cluster PostgreSQL service for non-production deployments:

<div class="tabs">

### Managed PostgreSQL (Recommended)

For production deployments, we recommend using a managed PostgreSQL service:

- [Google Cloud SQL](https://cloud.google.com/sql/docs/postgres/)
- [AWS RDS for PostgreSQL](https://aws.amazon.com/rds/postgresql/)
- [Azure Database for PostgreSQL](https://docs.microsoft.com/en-us/azure/postgresql/)
- [DigitalOcean Managed PostgreSQL](https://www.digitalocean.com/products/managed-databases-postgresql)

Ensure that your PostgreSQL service:

- Is running and accessible from your cluster
- Is in the same network/project as your cluster
- Has proper credentials and a database created for Coder

### In-Cluster PostgreSQL (Development/PoC)

For proof-of-concept deployments, you can use Bitnami Helm chart to install PostgreSQL in your Kubernetes cluster:

```console
helm repo add bitnami https://charts.bitnami.com/bitnami
helm install coder-db bitnami/postgresql \
    --namespace coder \
    --set auth.username=coder \
    --set auth.password=coder \
    --set auth.database=coder \
    --set persistence.size=10Gi
```

After installation, the cluster-internal database URL will be:

```text
postgres://coder:coder@coder-db-postgresql.coder.svc.cluster.local:5432/coder?sslmode=disable
```

For more advanced PostgreSQL management, consider using the
[Postgres operator](https://github.com/zalando/postgres-operator).

</div>

## Create the database connection secret

Create a Kubernetes secret with your PostgreSQL connection URL:

```shell
kubectl create secret generic coder-db-url -n coder \
  --from-literal=url="postgres://coder:coder@coder-db-postgresql.coder.svc.cluster.local:5432/coder?sslmode=disable"
```

> [!Important]
> If you're using a managed PostgreSQL service, replace the connection URL with your specific database credentials.

## Install Coder through the Rancher UI

![Coder installed on Rancher](../images/install/coder-rancher.png)

1. In the Rancher Manager console, select your target Kubernetes cluster for Coder.

1. Navigate to **Apps** > **Charts**

1. From the dropdown menu, select **Partners** and search for `Coder`

1. Select **Coder**, then **Install**

1. Select the `coder` namespace you created earlier and check **Customize Helm options before install**.

   Select **Next**

1. On the configuration screen, select **Edit YAML** and enter your Coder configuration settings:

   <details>
   <summary>Example values.yaml configuration</summary>

   ```yaml
   coder:
     # Environment variables for Coder
     env:
       - name: CODER_PG_CONNECTION_URL
         valueFrom:
           secretKeyRef:
             name: coder-db-url
             key: url

       # For production, uncomment and set your access URL
       # - name: CODER_ACCESS_URL
       #   value: "https://coder.example.com"

     # For TLS configuration (uncomment if needed)
     #tls:
     #  secretNames:
     #    - my-tls-secret-name
   ```

   For available configuration options, refer to the [Helm chart documentation](https://github.com/coder/coder/blob/main/helm#readme)
   or [values.yaml file](https://github.com/coder/coder/blob/main/helm/coder/values.yaml).

   </details>

1. Select a Coder version:

   - **Mainline**: `2.20.x`
   - **Stable**: `2.19.x`

   Learn more about release channels in the [Releases documentation](./releases.md).

1. Select **Next** when your configuration is complete.

1. On the **Supply additional deployment options** screen:

   1. Accept the default settings
   1. Select **Install**

1. A Helm install output shell will be displayed and indicates the installation status.

## Manage your Rancher Coder deployment

To update or manage your Coder deployment later:

1. Navigate to **Apps** > **Installed Apps** in the Rancher UI.
1. Find and select Coder.
1. Use the options in the **⋮** menu for upgrade, rollback, or other operations.

## Next steps

- [Create your first template](../tutorials/template-from-scratch.md)
- [Control plane configuration](../admin/setup/index.md)
