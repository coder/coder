# Deploy Coder on Rancher

You can deploy Coder on Rancher using a
[Workload](https://ranchermanager.docs.rancher.com/getting-started/quick-start-guides/deploy-workloads/nodeports).

## Requirements

- [SUSE Rancher Manager](https://ranchermanager.docs.rancher.com/getting-started/installation-and-upgrade/install-upgrade-on-a-kubernetes-cluster) running Kubernetes (K8s) 1.19+ with [SUSE Rancher Prime distribution](https://documentation.suse.com/cloudnative/rancher-manager/latest/en/integrations/kubernetes-distributions.html) (Rancher Manager 2.10+)
- Helm 3.5+ installed
- Workload Kubernetes cluster for Coder

## Install Coder with SUSE Rancher Manager

1. Create a namespace for the Coder control plane. In this tutorial, we call it `coder`:

   ```shell
   kubectl create namespace coder
   ```

1. Create a PostgreSQL instance:

   <div class="tabs">

   Coder does not manage a database server for you. This is required for storing
   data about your Coder deployment and resources.

   ### Managed PostgreSQL (recommended)

   If you're in a public cloud such as
   [Google Cloud](https://cloud.google.com/sql/docs/postgres/),
   [AWS](https://aws.amazon.com/rds/postgresql/),
   [Azure](https://docs.microsoft.com/en-us/azure/postgresql/), or
   [DigitalOcean](https://www.digitalocean.com/products/managed-databases-postgresql),
   you can use the managed PostgreSQL offerings they provide. Make sure that the
   PostgreSQL service is running and accessible from your cluster. It should be in
   the same network, same project, etc.

   ### In-Cluster PostgreSQL (for proof of concepts)

   You can install Postgres manually on your cluster using the
   [Bitnami PostgreSQL Helm chart](https://github.com/bitnami/charts/tree/master/bitnami/postgresql#readme).
   There are some [helpful guides](https://phoenixnap.com/kb/postgresql-kubernetes)
   on the internet that explain sensible configurations for this chart.

   Here's one example:

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

   You can optionally use the
   [Postgres operator](https://github.com/zalando/postgres-operator) to manage
   PostgreSQL deployments on your Kubernetes cluster.

   </div>

1. Create the PostgreSQL secret.

   Create a secret with the PostgreSQL database URL string. In the case of the
   self-managed PostgreSQL, the address will be:

   ```shell
   kubectl create secret generic coder-db-url -n coder \
     --from-literal=url="postgres://coder:coder@coder-db-postgresql.coder.svc.cluster.local:5432/coder?sslmode=disable"
   ```

1. Select the target workload K8s cluster for Coder in the Rancher Manager console and access the Kubectl shell.

1. From the Cluster Manager console, go to **Apps** > **Charts** and select **Partners**.

1. From the Chart providers, search for Coder.

1. Select **Coder**, then **Install**.

1. Select the target namespace you created for Coder and select **Customize Helm options before install**, then **Next**.

1. Configure Values used by Helm that help define the Coder App. Review step 4 from the standard Kubernetes installation for suggested values, then Next.

1. Accept the defaults on the last pane and select Install.

   A Helm install output shell will be displayed and should indicate success when completed.

1. To update a Coder deployment, select Coder from the Installed Apps and update as desired.

## Next steps

- [Create your first template](../tutorials/template-from-scratch.md)
- [Control plane configuration](../admin/setup/index.md)
