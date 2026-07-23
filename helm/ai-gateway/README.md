# Coder AI Gateway Helm chart

This chart deploys the Coder AI Gateway as a standalone Kubernetes Deployment.
The Gateway connects to Coder using `CODER_URL` and an AI Gateway key. To forward
proxied AI traffic to the standalone Gateway, configure the Coder AI Gateway
Proxy (`aibridgeproxyd`) after installing the chart.

The chart does not create credentials or TLS Secrets.

## Install

### Prerequisites

- An AI Gateway key created in Coder.
- A Coder image that includes the `coder ai-gateway start` command. The official
  Coder v2.36.0 image is the first version to include this command.

### Configure the chart

Create a `values.yaml` file with the Coder URL and an AI Gateway key source. The
following example uses a Kubernetes Secret in the Helm release namespace:

```console
kubectl create secret generic coder-ai-gateway-key \
  --namespace <release-namespace> \
  --from-literal=key='<AI gateway key>'
```

```yaml
coder:
  image:
    # Required when installing the chart directly from Git.
    tag: "<coder version>"
  env:
    - name: CODER_URL
      value: https://coder.example.com

aigateway:
  keySecret:
    name: coder-ai-gateway-key
```

The Gateway can also connect to Coder through an in-cluster Service, for
example:

```yaml
coder:
  env:
    - name: CODER_URL
      value: http://coder.coder.svc.cluster.local:80
```

For HTTPS, the Coder certificate must cover the internal Service hostname and
the Gateway must trust its issuing CA.

Instead of `aigateway.keySecret`, set `CODER_AI_GATEWAY_KEY` or
`CODER_AI_GATEWAY_KEY_FILE` through `coder.env`. Environment variables can also
be supplied through `coder.envFrom`. The chart does not check for variable
conflicts, regardless of whether values come from Helm options, `coder.env`, or
`coder.envFrom`.

When installing a released chart package, the chart automatically uses the
matching Coder image version. Set `coder.image.tag` only when installing
directly from Git or overriding the image version. Custom images must provide
the `coder ai-gateway start` command.

### Install the chart

```console
helm install ai-gateway ./helm/ai-gateway \
  --namespace <release-namespace> \
  --values values.yaml
```

## Connect Coder to the standalone Gateway

To route proxied AI requests through the standalone Gateway, configure the Coder
AI Gateway Proxy with a target URL. When `service.enable` is true, the chart
notes show the direct in-cluster Service URL, including the scheme selected by
`aigateway.listenerTLS`. Retrieve it with:

```console
helm get notes ai-gateway --namespace <release-namespace>
```

The chart notes do not show an Ingress or `HTTPRoute` URL. To route through one
of these entry points, set `CODER_AI_GATEWAY_PROXY_TARGET` to its URL instead.
When `service.enable` is false, set the target to the URL of your user-managed
route to the Deployment.

When listener TLS uses a private CA, the AI Gateway Proxy must trust that CA to
connect directly to the Service over HTTPS.

## TLS

For Gateway-to-Coder HTTPS with a private CA, set
`aigateway.coderTLS.caSecret`. If Coder requires client mTLS, also set
`aigateway.coderTLS.clientSecret`.

Prefer terminating client-facing TLS at a Kubernetes Ingress or a `Gateway`
resource from the Kubernetes Gateway API. To terminate TLS in the AI Gateway
process, set `aigateway.listenerTLS.name` to an existing TLS Secret.

Client-facing TLS and backend TLS are independent. The `ingress.tls` settings
configure TLS between clients and the Ingress. For `HTTPRoute`, the Gateway
listener that accepts client connections is configured outside this chart.
These settings do not configure whether the Ingress or Gateway connects to the
AI Gateway Service using HTTP or HTTPS.

When `aigateway.listenerTLS` is enabled behind an Ingress or `HTTPRoute`,
configure the entry point to connect to the Service using HTTPS and trust the AI
Gateway certificate. Ingress backend TLS is controller-specific and can usually
be configured with `ingress.annotations`. Gateway API backend TLS uses a
separate `BackendTLSPolicy`, which can be managed outside this chart or rendered
with `extraTemplates`. The chart does not infer or validate this
controller-specific configuration. Without backend TLS, the entry point sends
plaintext HTTP to the HTTPS listener, which typically results in a TLS handshake
error reported as HTTP 502.

All referenced TLS Secrets must exist in the Helm release namespace.

## Networking

The data-plane Service, which carries LLM traffic, is a `ClusterIP` by default.
`NodePort` and `LoadBalancer` are explicit alternatives. Ingress and `HTTPRoute`
are optional and both route to the data-plane Service. If you enable Ingress or
`HTTPRoute`, use a `ClusterIP` Service unless you intentionally need a second
external entry point through a `LoadBalancer` Service.

## Scaling and resources

Set `coder.replicaCount` to run multiple AI Gateway replicas. The default
resource requests are 1 CPU and 1 GiB of memory per replica. These requests are
a starting point, not a capacity guarantee. CPU and memory usage depend heavily
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
2. Update the configured key source:
   - For `aigateway.keySecret`, update the referenced Secret or set `name` to a
     new Secret.
   - For a key supplied through `coder.env`, update the environment variable or
     the file it references.
3. If the update did not trigger a rollout, restart the Deployment, for example:

   ```console
   kubectl rollout restart deployment/coder-ai-gateway \
     --namespace <release-namespace>
   ```

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
