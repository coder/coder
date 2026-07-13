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

When installing a released chart package, the chart automatically uses the
matching Coder image version. Set `coder.image.tag` only when installing
directly from Git or overriding the image version.

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

### Install the chart

```console
helm install ai-gateway ./helm/ai-gateway --values values.yaml
```

If AI Gateway Proxy is enabled and should forward intercepted requests to
this standalone Gateway instead of the embedded one, point coderd at the
Gateway Service created by this chart:

```text
CODER_AI_GATEWAY_PROXY_TARGET=http://coder-ai-gateway.<namespace>.svc.cluster.local:80
```

The exact value for your Helm release is shown after installation and can be
retrieved with `helm get notes ai-gateway`.

## TLS

For Gateway-to-Coder HTTPS with a private CA, set
`aigateway.coderTLS.caSecret`. If Coder requires client mTLS, also set
`aigateway.coderTLS.clientSecret`.

Prefer terminating client-facing TLS at a Kubernetes Ingress or a `Gateway`
resource from the Kubernetes Gateway API. To terminate TLS in the AI Gateway
process, configure `aigateway.listenerTLS.secretName`.

All referenced TLS Secrets must exist in the Helm release namespace.

## Networking and scaling

The data-plane Service, which carries LLM traffic, is a `ClusterIP` by
default. `NodePort` and `LoadBalancer` are explicit alternatives. Ingress and
HTTPRoute are optional and both route to the data-plane Service. If you enable
Ingress or HTTPRoute, use a `ClusterIP` Service unless you intentionally need a
second external entry point through a `LoadBalancer` Service.

Set `coder.replicaCount` to run multiple AI Gateway replicas. Manage resources
such as a Horizontal Pod Autoscaler or PodDisruptionBudget through your platform
configuration or `extraTemplates`.

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
release. Each YAML string can use Helm release values and chart helpers. Use it
for small companion resources, such as a `NetworkPolicy`.
