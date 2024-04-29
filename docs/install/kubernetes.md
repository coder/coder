## Requirements

Before proceeding, please ensure that you have a Kubernetes cluster running K8s
1.19+ and have Helm 3.5+ installed.

You'll also want to install the
[latest version of Coder](https://github.com/coder/coder/releases/latest)
locally in order to log in and manage templates.

> Coder supports two release channels: mainline for the true latest version of
> Coder, and stable for large enterprise deployments. Before installing your
> control plane via Helm, please read the [Releases](./releases.md) document to
> identify the best-suited release for your team, then specify the version using
> Helm's `--version` flag.

> The version flags for both stable and mainline are automatically filled in
> this page.

> If you need help setting up k8s, we have a
> [repo with Terraform configuration](https://github.com/ElliotG/coder-oss-tf)
> to provision Coder on Google GKE, Azure AKS, AWS EKS, DigitalOcean DOKS,
> IBMCloud K8s, OVHCloud K8s, and Scaleway K8s Kapsule.

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

   For the **mainline** Coder release:

   <!-- autoversion(mainline): "--version [version]" -->

   ```shell
   helm install coder coder-v2/coder \
       --namespace coder \
       --values values.yaml \
       --version 2.10.2
   ```

   For the **stable** Coder release:

   <!-- autoversion(stable): "--version [version]" -->

   ```shell
   helm install coder coder-v2/coder \
       --namespace coder \
       --values values.yaml \
       --version 2.9.4
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

## Kubernetes Security Reference

Below are common requirements we see from our enterprise customers when
deploying an application in Kubernetes. This is intended to serve as a
reference, and not all security requirements may apply to your business.

1. **All container images must be sourced from an internal container registry.**

   - Control plane - To pull the control plane image from the appropriate
     registry,
     [update this Helm chart value](https://github.com/coder/coder/blob/f57ce97b5aadd825ddb9a9a129bb823a3725252b/helm/coder/values.yaml#L43-L50).
   - Workspaces - To pull the workspace image from your registry,
     [update the Terraform template code here](https://github.com/coder/coder/blob/f57ce97b5aadd825ddb9a9a129bb823a3725252b/examples/templates/kubernetes/main.tf#L271).
     This assumes your cluster nodes are authenticated to pull from the internal
     registry.

2. **All containers must run as non-root user**

   - Control plane - Our control plane pod
     [runs as non-root by default](https://github.com/coder/coder/blob/f57ce97b5aadd825ddb9a9a129bb823a3725252b/helm/coder/values.yaml#L124-L127).
   - Workspaces - Workspace pod UID is
     [set in the Terraform template here](https://github.com/coder/coder/blob/f57ce97b5aadd825ddb9a9a129bb823a3725252b/examples/templates/kubernetes/main.tf#L274-L276),
     and are not required to run as `root`.

3. **Containers cannot run privileged**

   - Coder's control plane does not run as privileged.
     [We disable](https://github.com/coder/coder/blob/f57ce97b5aadd825ddb9a9a129bb823a3725252b/helm/coder/values.yaml#L141)
     `allowPrivilegeEscalation`
     [by default](https://github.com/coder/coder/blob/f57ce97b5aadd825ddb9a9a129bb823a3725252b/helm/coder/values.yaml#L141).
   - Workspace pods do not require any elevated privileges, with the exception
     of our `envbox` workspace template (used for docker-in-docker workspaces,
     not required).

4. **Containers cannot mount host filesystems**

   - Both the control plane and workspace containers do not require any host
     filesystem mounts.

5. **Containers cannot attach to host network**

   - Both the control plane and workspaces use the Kubernetes networking layer
     by default, and do not require host network access.

6. **All Kubernetes objects must define resource requests/limits**

   - Both the control plane and workspaces set resource request/limits by
     default.

7. **All Kubernetes objects must define liveness and readiness probes**

   - Control plane - The control plane Deployment has liveness and readiness
     probes
     [configured by default here](https://github.com/coder/coder/blob/f57ce97b5aadd825ddb9a9a129bb823a3725252b/helm/coder/templates/_coder.tpl#L98-L107).
   - Workspaces - the Kubernetes Deployment template does not configure
     liveness/readiness probes for the workspace, but this can be added to the
     Terraform template, and is supported.

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
