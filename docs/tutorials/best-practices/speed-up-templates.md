# Speed up your Coder templates and workspaces

October 31, 2024

---

If it takes your workspace a long time to start, find out why and make some
changes to your Coder templates to help speed things up.

## Monitoring

You can monitor [Coder logs](../../admin/monitoring/logs.md) through the
system-native tools on your deployment platform, or stream logs to tools like
Splunk, Datadog, Grafana Loki, and others.

### Coder Observability Chart

Use the [Observability Helm chart](https://github.com/coder/observability) for a
pre-built set of monitoring integrations.

With the Observability Helm chart, you can monitor [which specific startup
metrics are a part of the Helm chart?]

To install it with Helm:

```shell
helm repo add coder-observability https://helm.coder.com/observability
helm upgrade --install coder-observability coder-observability/coder-observability --version 0.1.1 --namespace coder-observability --create-namespace
```

### Enable Prometheus metrics for Coder

Our observability bundle gives you this for free which is nice and has
instructions on how to do it with Coder

[Prometheus.io](https://prometheus.io/docs/introduction/overview/#what-is-prometheus)
is included as part of the [observability chart](#coder-observability-chart).It
offers a variety of
[available metrics](../../admin/integrations/prometheus.md#available-metrics).

You can
[install it separately](https://prometheus.io/docs/prometheus/latest/getting_started/)
if you prefer.

### Workspace build timeline

Use the **Build timeline** to monitor the time it takes to start specific
workspaces. Identify long scripts, resources, and other things you can
potentially optimize within the template.

![Screenshot of a workspace and its build timeline](../../images/best-practice/build-timeline.png)

Adjust this request to match your Coder access URL and workspace:

```shell
curl -X GET http://coder-server:8080/api/v2/workspacebuilds/{workspacebuild}/timings \
  -H 'Accept: application/json' \
  -H 'Coder-Session-Token: API_KEY'
```

Visit the
[API documentation](../../reference/api/builds.md#get-workspace-build-timings-by-id)
for more information.

## Provisioners

`coder server` defaults to three provisioner daemons. Each provisioner daemon
can handle one single job, such as start, stop, or delete at a time and can be
resource intensive. When all provisioners are busy, workspaces enter a "pending"
state until a provisioner becomes available.

### Increase provisioner daemons

Provisioners are queue-based to reduce unpredictable load to the Coder server.
However, they can be scaled up to allow more concurrent provisioners. You risk
overloading the central Coder server if you use too many local provisioners, so
we recommend a maximum of five provisioners. For more than five provisioners, we
recommend that you move to [external provisioners](../../admin/provisioners.md).

If you can’t move to external provisioners, use the `provisioner-daemons` flag
to increase the number of provisioner daemons to five:

```shell
coder server --provisioner-daemons=5
```

Visit the
[CLI documentation](../../reference/cli/server.md#--provisioner-daemons) for
more information about increasing provisioner daemons and other options.

### Adjust provisioner CPU/memory

We recommend that you deploy Coder to its own respective Kubernetes cluster,
separate from production applications. Keep in mind that Coder runs development
workloads, so the cluster should be deployed as such, without production-level
configurations.

Adjust the CPU and memory values as shown in
[Helm provisioner values.yaml](https://github.com/coder/coder/blob/main/helm/provisioner/values.yaml#L134-L141):

```yaml
…
  resources:
    {}
    # limits:
    #   cpu: 2000m
    #   memory: 4096Mi
    # requests:
    #   cpu: 2000m
    #   memory: 4096Mi
…
```

Visit the
[validated architecture documentation](../../admin/infrastructure/validated-architectures/index.md#workspace-nodes)
for more information.

## Set up Terraform Provider Caching

Use `terraform init` to cache providers:

Pull the templates to your local device:

```shell
coder templates pull
```

Run `terraform init` to initialize the directory:

```shell
terraform init
```

Push the templates back to your Coder deployment:

```shell
coder templates push
```
