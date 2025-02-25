# Deploy Coder on Rancher

You can deploy Coder on Rancher using a
[Workload](https://ranchermanager.docs.rancher.com/getting-started/quick-start-guides/deploy-workloads/nodeports).

## Requirements

- [Rancher](https://ranchermanager.docs.rancher.com/getting-started/installation-and-upgrade/install-upgrade-on-a-kubernetes-cluster)
  - alternative link: [Deploy Rancher Manager](https://ranchermanager.docs.rancher.com/getting-started/quick-start-guides/deploy-rancher-manager)
- other requirements

## Configure Rancher

The first thing to do

## Install Coder with Helm

```shell
helm repo add coder-v2 https://helm.coder.com/v2
```

Create a `values.yaml` with the configuration settings you'd like for your
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

You can watch Coder start up by running `kubectl get pods -n coder`. Once Coder
has started, the `coder-*` pods should enter the `Running` state.

## Log in to Coder

Use `kubectl get svc -n coder` to get the IP address of the LoadBalancer. Visit
this in the browser to set up your first account.

If you do not have a domain, you should set `CODER_ACCESS_URL` to this URL in
the Helm chart and upgrade Coder (see below). This allows workspaces to connect
to the proper Coder URL.

## Upgrading Coder in Rancher

To upgrade Coder in the future or change values, you can run the following
command:

```shell
helm repo update
helm upgrade coder coder-v2/coder \
  --namespace coder \
  -f values.yaml
```

## Coder Observability Chart

Use the [Observability Helm chart](https://github.com/coder/observability) for a
pre-built set of dashboards to monitor your control plane over time. It includes
Grafana, Prometheus, Loki, and Alert Manager out-of-the-box, and can be deployed
on your existing Grafana instance.

We recommend that all administrators deploying on Kubernetes set the
observability bundle up with the control plane from the start. For installation
instructions, visit the
[observability repository](https://github.com/coder/observability?tab=readme-ov-file#installation).

## Next steps

- [Create your first template](../tutorials/template-from-scratch.md)
- [Control plane configuration](../admin/setup/index.md)
