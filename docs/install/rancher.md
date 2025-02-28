# Deploy Coder on Rancher

You can deploy Coder on Rancher as a
[Workload](https://ranchermanager.docs.rancher.com/getting-started/quick-start-guides/deploy-workloads/workload-ingress).

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

   Optionally, you can use the
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

1. From the **Cluster Manager** console, go to **Apps** > **Charts**

1. Select **Partners** from the drop-down menu and search for `Coder`.

1. Select **Coder**, then **Install**.

1. Select the target namespace you created for Coder and select **Customize Helm options before install**, then **Next**.

1. Configure Values used by Helm that help define the Coder App.

   Select **Edit YAML** and enter configuration settings for your deployment.

   <details><summary>Expand for an example `values.yaml`</summary>

   <!-- from kubernetes.md -->

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
       # - name: CODER_ACCESS_URL
       #   value: "https://coder.example.com"

     #tls:
     #  secretNames:
     #    - my-tls-secret-name
   ```

   > You can view our
   > [Helm README](https://github.com/coder/coder/blob/main/helm#readme) for
   > details on the values that are available, or you can view the
   > [values.yaml](https://github.com/coder/coder/blob/main/helm/coder/values.yaml)
   > file directly.

   We support two release channels: mainline and stable - read the
   [Releases](./releases.md) page to learn more about which best suits your team.

   - **Mainline** Coder release:

     <!-- autoversion(mainline): "--version [version]" -->

     ```shell
     helm install coder coder-v2/coder \
         --namespace coder \
         --values values.yaml \
         --version 2.19.0
     ```

   - **Stable** Coder release:

     <!-- autoversion(stable): "--version [version]" -->

     ```shell
     helm install coder coder-v2/coder \
         --namespace coder \
         --values values.yaml \
         --version 2.18.5
     ```

   </details>

   Select **Next** when you're done.

1. On the **Supply additional deployment options** screen, accept the default settings, then select **Install**.

   A Helm install output shell will be displayed and should indicate success when completed.

In the future, if you need to update a Coder deployment, select Coder from **Installed Apps** and use the options in the **⋮** menu.

## Next steps

- [Create your first template](../tutorials/template-from-scratch.md)
- [Control plane configuration](../admin/setup/index.md)
