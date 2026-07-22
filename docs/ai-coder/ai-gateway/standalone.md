# Deploy AI Gateway as a standalone service

When AI traffic needs dedicated compute, independent scaling, or a separate network endpoint, you can deploy AI Gateway separately from the Coder control plane (`coderd`).

A standalone AI Gateway serves client traffic on its own listener and maintains a control connection to `coderd`.
`coderd` continues to manage authentication, authorization, provider configuration, and AI session records.

## Before you begin

Standalone AI Gateway requires:

- Coder v2.36.0 or later.
- A Coder license with the [AI Governance Add-On](../ai-governance.md).
- AI Gateway enabled on the Coder control plane with `CODER_AI_GATEWAY_ENABLED=true` or `--ai-gateway-enabled=true`.
- The full Coder image or a Coder binary that includes the `coder ai-gateway start` command.

`coder ai-gateway start` does not read `CODER_AI_GATEWAY_ENABLED` from the standalone process.
However, this setting must remain enabled on `coderd` for Gateway key management and standalone control connections.

## Create a Gateway key

Each standalone replica uses a Gateway key to authenticate and establish its control connection to `coderd`.
Gateway key management requires site-level `ai_gateway_key` permissions, which the built-in Owner role includes by default.
A site-level custom role can also manage keys when granted the corresponding permissions.

Log in to the Coder CLI as an Owner or another user with these permissions, then create a dedicated key for the standalone deployment:

```console
coder login https://coder.example.com
coder ai-gateway keys create standalone-production
```

Save the key from the command output immediately because Coder does not display the plaintext value again.

A single key can authenticate multiple replicas in the same deployment.
For independent rotation and revocation, use a separate key for each standalone deployment or environment.

## Start a standalone process

Set the Coder URL, Gateway key, and listener address, then start the Gateway:

```console
export CODER_URL=https://coder.example.com
export CODER_AI_GATEWAY_KEY='<AI Gateway key>'
export CODER_AI_GATEWAY_HTTP_ADDRESS=0.0.0.0:4001
coder ai-gateway start
```

Use `CODER_AI_GATEWAY_KEY_FILE` instead of `CODER_AI_GATEWAY_KEY` to read the key from a file.
The standalone process does not require a user login or `CODER_SESSION_TOKEN` after you provide the Gateway key.

The standalone Gateway fetches provider configuration from `coderd`.
Configure at least one [AI provider](./providers.md) in Coder before sending provider traffic through the Gateway.
The standalone Gateway does not use the deprecated [provider seed variables](./providers.md#database-management-of-providers).

The listener uses HTTP by default.
Set both `CODER_AI_GATEWAY_TLS_CERT_FILE` and `CODER_AI_GATEWAY_TLS_KEY_FILE` to terminate TLS in the process.
For all command options, refer to [`coder ai-gateway start`](../../reference/cli/ai-gateway_start.md).

## Run AI Gateway in Kubernetes

To run the standalone Gateway as a Kubernetes workload, provide the same environment variables and network access to the Coder URL.
You can manage the workload with your own Kubernetes manifests or use the provided Helm chart.
The chart configures the Deployment, probes, Service, and Ingress.

To use the Kubernetes examples, install `kubectl` and configure access to the cluster where AI Gateway will run.
Install Helm if you use the chart.

Create a namespace and store the Gateway key in a Kubernetes Secret:

```console
kubectl create namespace coder-ai-gateway
kubectl create secret generic coder-ai-gateway-key \
  --namespace coder-ai-gateway \
  --from-literal=key='<AI Gateway key>'
```

### Configure the Helm chart

Create `ai-gateway-values.yaml` with at least the Coder URL and key Secret:

```yaml
coder:
  env:
    - name: CODER_URL
      value: https://coder.example.com

aigateway:
  keySecret:
    name: coder-ai-gateway-key
```

The Gateway must be able to reach the Coder URL from every replica.
For an HTTPS URL signed by a private CA, configure `aigateway.coderTLS.caSecret`.
If Coder requires client mTLS, also configure `aigateway.coderTLS.clientSecret`.

The chart defaults to one replica, a `ClusterIP` Service, and a data-plane listener on port 4001.
It also enables a separate Prometheus listener on port 2112.

For the complete chart configuration, including private CAs, mTLS, Ingress, Kubernetes Gateway API, and additional manifests, see the [AI Gateway Helm chart README](../../../helm/ai-gateway/README.md).

### Install the Helm chart

Install the chart version that matches the Coder control plane from GitHub Container Registry:

```console
helm install ai-gateway \
  oci://ghcr.io/coder/chart/coder-ai-gateway \
  --namespace coder-ai-gateway \
  --values ai-gateway-values.yaml \
  --version '<Coder version>'
```

When you install the chart directly from a Git checkout, set `coder.image.tag` explicitly and install `./helm/ai-gateway`.
Released chart packages select the matching Coder image by default.
If you need to run different `coderd` and Gateway versions, refer to [Version compatibility](#version-compatibility).

### Validate the Helm deployment

Wait for the Deployment to become available:

```console
kubectl rollout status deployment/coder-ai-gateway \
  --namespace coder-ai-gateway \
  --timeout=5m
```

The chart configures the following probes by default:

- The liveness probe requests `/healthz` and verifies that the HTTP listener is serving.
- The readiness probe requests `/readyz` and verifies that the Gateway is connected to `coderd` and has completed its initial provider load.
- The startup probe is disabled by default.

`/healthz` can return HTTP 200 while `/readyz` returns HTTP 503.
This occurs during the initial connection to `coderd`, during the initial provider load, or while the control connection is unavailable.
After the initial provider load succeeds, readiness returns when the connection to `coderd` recovers.

Port-forward the Service to inspect both endpoints:

```console
kubectl port-forward \
  --namespace coder-ai-gateway \
  service/coder-ai-gateway \
  4001:80
```

In another terminal, run:

```console
curl --fail http://127.0.0.1:4001/healthz
curl --fail http://127.0.0.1:4001/readyz
```

List Gateway keys and verify that the key has a recent heartbeat:

```console
coder ai-gateway keys list
```

The first heartbeat is recorded when the replica connects.
Active sessions update the timestamp every 60 seconds.

## Route traffic to the standalone Gateway

The standalone deployment does not move traffic automatically.
Configure AI Gateway Proxy and direct AI clients to send requests to the standalone endpoint.

### AI Gateway Proxy

[AI Gateway Proxy](./ai-gateway-proxy/index.md) remains part of the `coder server` process.
Configure its target to use the standalone Service, Ingress, `HTTPRoute`, or load balancer.

If you installed the Helm chart, get the exact in-cluster Service URL from the chart notes:

```console
helm get notes ai-gateway --namespace coder-ai-gateway
```

If you use your own Kubernetes manifests, use the URL of the Service you created.
Set the Service URL on the Coder control plane, for example:

```sh
CODER_AI_GATEWAY_PROXY_TARGET=http://coder-ai-gateway.coder-ai-gateway.svc.cluster.local:80
```

Restart or upgrade the Coder deployment after changing this setting.
The proxy appends the provider name and request path to the target, so do not include query parameters.

### Direct AI clients

Clients that use `<Coder access URL>/api/v2/ai-gateway/<provider-name>/` continue to reach the embedded Gateway.
To route these clients to the standalone Gateway, replace the Coder access URL and embedded route prefix with the standalone endpoint while keeping the provider path.

For example, if the Gateway is reachable at `https://ai-gateway.example.com` and the providers are configured, configure Claude Code with:

```sh
ANTHROPIC_BASE_URL=https://ai-gateway.example.com/anthropic
```

Configure an OpenAI-compatible client with:

```sh
OPENAI_BASE_URL=https://ai-gateway.example.com/openai/v1
```

The standalone listener also accepts the equivalent `/api/v2/ai-gateway/<provider-name>/` paths for compatibility.

### Expose the standalone AI Gateway

If clients cannot access the in-cluster Service, expose the Gateway through an Ingress, an `HTTPRoute`, or a load balancer Service.
In the provided Helm chart, set:

```yaml
service:
  type: LoadBalancer
```

Use TLS to protect credentials and AI traffic whenever they cross an untrusted network.
Prefer TLS termination at the Ingress or Kubernetes Gateway when your platform already manages certificates there.
To terminate TLS in the AI Gateway process, reference an existing TLS Secret with `aigateway.listenerTLS`.

If `CODER_AI_GATEWAY_PROXY_TARGET` uses HTTPS with a private CA, add that CA to the trust store of the `coderd` pods before changing the proxy target.
For the Coder Helm chart, you can mount a CA bundle with `coder.certs.secrets`.
This trust configuration is separate from `aigateway.coderTLS.caSecret`, which configures the connection from the standalone Gateway to `coderd`.

## Scale replicas

Run multiple replicas behind a Service or load balancer.
In the provided Helm chart, set:

```yaml
coder:
  replicaCount: 3
```

The Service balances requests across ready replicas.
Each replica maintains its own control connection, local caches, concurrency limits, metrics, logs, and traces.
All replicas fetch provider configuration from the same Coder deployment and write AI session records through `coderd`.
Sticky load balancing can improve cache efficiency, but it is not required for correctness.

When you enable [API dumps](./setup.md#api-dumps), each replica writes dumps to its own local disk.
Use persistent storage if you need those files to survive pod replacement.

The chart requests 1 CPU and 1 GiB of memory per replica and does not set resource limits.
Measure production traffic before changing requests, limits, or `CODER_AI_GATEWAY_MAX_CONCURRENCY`.
The chart does not create a Horizontal Pod Autoscaler or PodDisruptionBudget.

## Migrate from the embedded Gateway

Use a gradual cutover so that you can validate the standalone data plane before moving production traffic:

1. Upgrade `coderd` to the target Coder version.
1. Keep `CODER_AI_GATEWAY_ENABLED=true` on `coderd`.
1. Create a dedicated Gateway key.
1. Deploy one standalone replica without changing client or proxy routing.
1. Verify `/readyz`, the key heartbeat, metrics, logs, and traces.
1. Send a test request directly to the standalone Gateway and confirm that the AI session appears in Coder.
1. Set `CODER_AI_GATEWAY_PROXY_TARGET` to the standalone endpoint.
1. Update direct client base URLs that should use the standalone endpoint.
1. Scale the standalone deployment after the canary path is stable.

Keep `CODER_AI_GATEWAY_ENABLED=true` on `coderd` after the cutover.
The setting is required for standalone control connections and Coder features that use the in-process Gateway.

## Roll back to the embedded Gateway

To return traffic to the embedded Gateway:

1. Remove `CODER_AI_GATEWAY_PROXY_TARGET`, or set it to `<Coder access URL>/api/v2/ai-gateway`.
1. Restart or upgrade `coderd` so the proxy target change takes effect.
1. Restore direct client base URLs to `<Coder access URL>/api/v2/ai-gateway/<provider-name>/`.
1. Verify requests and AI session records through the embedded route.
1. Scale down or uninstall the standalone deployment.
1. Delete the standalone Gateway key after all replicas have stopped.

Delete the key with:

```console
coder ai-gateway keys delete standalone-production
```

Deleting a key prevents new connections immediately.
When an existing connection detects the deletion during a later heartbeat, it closes, and the standalone Gateway attempts to reconnect.
Stop the deployment before deleting its key.

## Version compatibility

The standalone Gateway image does not need to match the `coderd` image version exactly.
The components can connect when their AI Gateway API versions are compatible, and `coderd` checks compatibility whenever a Gateway replica connects.
For the compatibility rules and the rejection response, refer to [Version compatibility](./reference.md#version-compatibility) in the AI Gateway reference.

A Gateway replica can run behind `coderd`, but never ahead of it, so sequence changes that move both components:

- To upgrade, upgrade `coderd` first, then roll out the standalone Gateway release.
- To roll back, roll back the standalone Gateway first, then roll back `coderd`.

Running the same Coder release on `coderd` and every standalone replica is the simplest way to stay compatible, but it is not required.
