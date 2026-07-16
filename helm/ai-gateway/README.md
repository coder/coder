# Coder AI Gateway Helm chart

This chart runs the Coder AI Gateway as an independent Kubernetes Deployment.
It references an existing AI Gateway key Secret.
It does not create credentials or TLS Secrets.

## Install

### Prerequisites

1. An AI Gateway key created in Coder.
2. A Kubernetes Secret containing that key in the chart namespace:

   ```console
   kubectl create secret generic coder-ai-gateway-key \
     --namespace <release-namespace> \
     --from-literal=key='<AI gateway key>'
   ```

### Configure the chart

Create a `values.yaml` file with the required Gateway key Secret and Coder
connection:

```yaml
coder:
  image:
    # Required when installing the chart directly from Git.
    tag: "<coder version>"

aigateway:
  # Existing Secret created in the chart namespace.
  keySecret:
    name: coder-ai-gateway-key

  # URL used by the Gateway to connect to Coder.
  coderURL: https://coder.example.com
```

Alternatively, the Gateway can connect to Coder through an in-cluster Service.
Replace `coderURL` with:

```yaml
aigateway:
  coderService:
    # Defaults to coder.
    name: coder
    # Defaults to the Helm release namespace.
    namespace: coder
    # Required: http or https.
    scheme: http
    # Defaults to 80.
    port: 80
```

The chart constructs
`<scheme>://<name>.<namespace>.svc.cluster.local:<port>`. For HTTPS, the Coder
certificate must cover the internal Service hostname and the Gateway must trust
its issuing CA.

When installing a released chart package, the chart automatically uses the
matching Coder image version. Set `coder.image.tag` only when installing
directly from Git or overriding the image version.

The chart rejects chart-owned variable names in `coder.env`. Helm cannot inspect
keys supplied by `coder.envFrom`, do not include chart-owned variables there.
In particular, `CODER_AI_GATEWAY_KEY` conflicts with the chart-managed
`CODER_AI_GATEWAY_KEY_FILE` variable and prevents startup.

### Install the chart

```console
helm install ai-gateway ./helm/ai-gateway \
  --namespace <release-namespace> \
  --values values.yaml
```

If AI Gateway Proxy is enabled and should forward intercepted requests to
this standalone Gateway instead of the embedded one, point coderd at the
Gateway Service created by this chart. The exact target, including the scheme
selected by `aigateway.listenerTLS`, is shown after installation and can be
retrieved with `helm get notes ai-gateway`.

When `service.enable` is false, configure `CODER_AI_GATEWAY_PROXY_TARGET` with
the URL of your user-managed route to the deployment.

## TLS

For Gateway-to-Coder HTTPS with a private CA, set
`aigateway.coderTLS.caSecret`. If Coder requires client mTLS, also set
`aigateway.coderTLS.clientSecret`.

Prefer terminating client-facing TLS at a Kubernetes Ingress or a `Gateway`
resource from the Kubernetes Gateway API. To terminate TLS in the AI Gateway
process, configure `aigateway.listenerTLS.name`.

Client-facing TLS and backend TLS are independent. The `ingress.tls` settings
configure TLS between clients and the Ingress. For HTTPRoute, the Gateway
listener that accepts client connections is configured outside this chart.
These client-facing settings do not configure whether the Ingress or Gateway
connects to the AI Gateway Service using HTTP or HTTPS.

When `aigateway.listenerTLS` is enabled behind an Ingress or HTTPRoute, configure
the entry point to connect to the Service using HTTPS and to trust the AI
Gateway certificate. Ingress backend TLS is controller-specific and can usually
be configured with `ingress.annotations`. Gateway API backend TLS is configured
with a separate `BackendTLSPolicy`, which can be managed outside this chart or
rendered with `extraTemplates`. The chart does not infer or validate this
controller-specific backend configuration.

All referenced TLS Secrets must exist in the Helm release namespace.

## Networking and scaling

The data-plane Service, which carries LLM traffic, is a `ClusterIP` by
default. `NodePort` and `LoadBalancer` are explicit alternatives. Ingress and
HTTPRoute are optional and both route to the data-plane Service. If you enable
Ingress or HTTPRoute, use a `ClusterIP` Service unless you intentionally need a
second external entry point through a `LoadBalancer` Service.

Set `coder.replicaCount` to run multiple AI Gateway replicas. The default
resource requests are 1 CPU and 1 GiB of memory per replica. These requests are
a starting point, not a capacity guarantee. CPU and Memory usage depends heavily
on concurrent requests and payload size.

Adjust `coder.resources` after observing production traffic. Consider setting
`CODER_AI_GATEWAY_MAX_CONCURRENCY` through `coder.env` to bound concurrent
requests per replica. The application default is unlimited. The chart does not
set resource limits by default, which avoids CPU throttling and fixed memory
limits for bursty workloads. Manage resources such as a Horizontal Pod
Autoscaler or PodDisruptionBudget through your platform configuration or
`extraTemplates`.

## Metrics

Every pod runs an unauthenticated metrics listener on the named `metrics` port,
which maps to port `2112`. The chart does not create monitoring discovery
resources. Configure pod-based discovery with `coder.podAnnotations`, for
example:

```yaml
coder:
  podAnnotations:
    prometheus.io/scrape: "true"
    prometheus.io/port: "2112"
```

Alternatively, create discovery resources such as a `ServiceMonitor` through
your monitoring stack or `extraTemplates`.

## Key rotation

1. Create a new AI Gateway key in Coder.
2. Update the Kubernetes Secret.
3. Restart the Deployment, for example with
   `kubectl rollout restart deployment/coder-ai-gateway`.
4. Verify every replica is ready and serving with the new key.
5. Revoke the old key.

Secret updates do not change the Deployment pod template automatically. A
reloader controller can be configured through `coder.annotations` or
`coder.podAnnotations`.

## Extra manifests

`extraTemplates` renders additional Kubernetes manifests as part of the Helm
release. Entries can be YAML strings or Kubernetes objects, and can use Helm
release values and chart helpers. Use them for small companion resources, such
as a `NetworkPolicy`.
