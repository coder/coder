# Deploy AI Gateway as a standalone service

> [!NOTE]
> AI Gateway requires a [Premium](../ai-governance.md) license.
> Community deployments cannot access AI Gateway.

When AI traffic needs dedicated compute, independent scaling, or a separate network endpoint, you can deploy AI Gateway separately from the Coder control plane (`coderd`).

A standalone AI gateway serves client traffic on its own listener and maintains a control connection to `coderd`.
`coderd` continues to manage authentication, authorization, provider configuration, and AI session records.

## Before you begin

A standalone AI gateway requires the following:

- Coder v2.36.0 or later.
- A [Premium license with AI Governance](../ai-governance.md).
- AI Gateway enabled on the Coder control plane with `CODER_AI_GATEWAY_ENABLED=true` or `--ai-gateway-enabled=true`.
- The full Coder image or a Coder binary that includes the `coder ai-gateway start` command.

`coder ai-gateway start` does not read `CODER_AI_GATEWAY_ENABLED` from the standalone process.
However, this setting must remain enabled on `coderd` for gateway key management and standalone control connections.

## Create a gateway key

Each standalone replica uses a gateway key to authenticate and establish its control connection to `coderd`.
Gateway key management requires site-level `ai_gateway_key` permissions, which only the built-in **owner** role includes.
Coder custom roles are organization-scoped and cannot grant site-level permissions.

Log in to the Coder CLI as an **owner**, then create a dedicated key for the standalone deployment:

```sh
coder login https://coder.example.com
coder ai-gateway keys create standalone-production
```

Save the key from the command output immediately because Coder does not display the plaintext value again.

A single key can authenticate multiple replicas in the same deployment.
For independent rotation and revocation, use a separate key for each standalone deployment or environment.

## Start a standalone process

Set the Coder URL, gateway key, and listener address, then start the gateway:

```sh
export CODER_URL=https://coder.example.com
export CODER_AI_GATEWAY_KEY='<gateway_key>'
export CODER_AI_GATEWAY_HTTP_ADDRESS=0.0.0.0:4001
coder ai-gateway start
```

Use `CODER_AI_GATEWAY_KEY_FILE` instead of `CODER_AI_GATEWAY_KEY` to read the key from a file.
The standalone process does not require a user login or `CODER_SESSION_TOKEN` after you provide the gateway key.

The listener defaults to `127.0.0.1:4001`, which accepts connections only from the local host.
Set `CODER_AI_GATEWAY_HTTP_ADDRESS` to a routable address, as shown above, before other hosts or pods can reach the gateway.

The standalone gateway fetches provider configuration from `coderd`.
Configure at least one [AI provider](./providers.md) in Coder before sending provider traffic through the gateway.
The standalone gateway does not use the deprecated [provider seed variables](./providers.md#database-management-of-providers).

The listener uses HTTP by default.
Set both `CODER_AI_GATEWAY_TLS_CERT_FILE` and `CODER_AI_GATEWAY_TLS_KEY_FILE` to terminate TLS in the process.
For all command options, refer to [`coder ai-gateway start`](../../reference/cli/ai-gateway_start.md).

## Run AI Gateway in Kubernetes

To run the standalone gateway as a Kubernetes workload, provide the same environment variables and network access to the Coder URL.
You can manage the workload with your own Kubernetes manifests or use the provided Helm chart.
The chart configures the Deployment, probes, and Service, plus an optional Ingress or `HTTPRoute`.

To use the Kubernetes examples, install `kubectl` and configure access to the cluster where AI Gateway will run.
Install Helm if you use the chart.

Create a namespace and store the gateway key in a Kubernetes Secret:

```sh
kubectl create namespace coder-ai-gateway
kubectl create secret generic coder-ai-gateway-key \
  --namespace coder-ai-gateway \
  --from-literal=key='<gateway key>'
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

The gateway must be able to reach the Coder URL from every replica.
For an HTTPS URL signed by a private CA, set `aigateway.coderTLS.caSecret.name` to the Secret holding the CA bundle.
If Coder requires client mTLS, also set `aigateway.coderTLS.clientSecret.name`.

The chart defaults to one replica, a `ClusterIP` Service, and a data-plane listener on port 4001.
It also enables a separate Prometheus listener on port 2112.
Ingress and `HTTPRoute` are disabled by default.

For the complete chart configuration, including private CAs, mTLS, Ingress, Kubernetes Gateway API, and additional manifests, refer to the [AI Gateway Helm chart README](../../../helm/ai-gateway/README.md).

### Install the Helm chart

Install the chart version that matches the Coder control plane from GitHub Container Registry:

```sh
helm install ai-gateway \
  oci://ghcr.io/coder/chart/coder-ai-gateway \
  --namespace coder-ai-gateway \
  --values ai-gateway-values.yaml \
  --version '<chart version>'
```

Chart versions omit the leading `v`, so use `2.36.0` rather than `v2.36.0`.

When you install the chart directly from a Git checkout, set `coder.image.tag` explicitly and install `./helm/ai-gateway`.
Released chart packages select the matching Coder image by default.
If you need to run different `coderd` and gateway versions, refer to [Version compatibility](#version-compatibility).

### Validate the Helm deployment

Wait for the Deployment to become available:

```sh
kubectl rollout status deployment/coder-ai-gateway \
  --namespace coder-ai-gateway \
  --timeout=5m
```

The chart configures the following probes by default:

- The liveness probe requests `/healthz` and verifies that the HTTP listener is serving.
- The readiness probe requests `/readyz` and verifies that the gateway is connected to `coderd` and has completed its initial provider load.
- The startup probe is disabled by default.

`/healthz` can return HTTP 200 while `/readyz` returns HTTP 503.
This occurs while the initial connection to `coderd` is being established, before the initial provider load, and whenever the control connection drops.
A replica can only become ready after its initial provider load succeeds, and it returns to ready whenever the control connection recovers.
Kubernetes removes an unready replica from the Service, so clients normally stop reaching it.
A request that still arrives waits for the control connection instead of failing.

Port-forward the Service to inspect both endpoints:

```sh
kubectl port-forward \
  --namespace coder-ai-gateway \
  service/coder-ai-gateway \
  4001:80
```

In another terminal, run:

```sh
curl --fail http://127.0.0.1:4001/healthz
curl --fail http://127.0.0.1:4001/readyz
```

Both endpoints return HTTP 200 with an empty body when the replica is serving and ready.

List gateway keys and verify that the key has a recent heartbeat:

```sh
coder ai-gateway keys list
```

The `LAST HEARTBEAT AT` column holds the timestamp.
The first heartbeat is recorded when the replica connects.
An active control connection updates the timestamp every 60 seconds.
Coder stores one timestamp per key, so a recent heartbeat on a shared key does not confirm that every replica is connected.
Check `/readyz` on each replica to verify individual health.

## Route traffic to the standalone gateway

The standalone deployment does not move traffic automatically.
Configure AI Gateway Proxy and direct AI clients to send requests to the standalone endpoint.

### AI Gateway Proxy

[AI Gateway Proxy](./ai-gateway-proxy/index.md) remains part of the `coder server` process.
Configure its target to use the standalone Service, Ingress, `HTTPRoute`, or load balancer.

If you installed the Helm chart, get the exact in-cluster Service URL from the chart notes:

```sh
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

Clients that use `<Coder access URL>/api/v2/ai-gateway/<provider-name>/` continue to reach the embedded gateway.
To route these clients to the standalone gateway, replace the Coder access URL and the embedded `/api/v2/ai-gateway` path prefix with the standalone endpoint while keeping the provider path.

For example, if the gateway is reachable at `https://ai-gateway.example.com` and the providers are configured, configure Claude Code with the following:

```sh
ANTHROPIC_BASE_URL=https://ai-gateway.example.com/anthropic
```

Configure an OpenAI-compatible client with the following:

```sh
OPENAI_BASE_URL=https://ai-gateway.example.com/openai/v1
```

For full per-client configuration examples, refer to [Client Configuration](./clients/index.md).

### Expose the standalone AI gateway

If clients cannot access the in-cluster Service, expose the gateway through an Ingress, an `HTTPRoute`, or a load balancer Service.
In the provided Helm chart, set the following:

```yaml
service:
  type: LoadBalancer
```

Use TLS to protect credentials and AI traffic whenever they cross an untrusted network.
Prefer TLS termination at the Ingress or Kubernetes Gateway when your platform already manages certificates there.
To terminate TLS in the gateway process, set `aigateway.listenerTLS.name` to an existing TLS Secret.

If `CODER_AI_GATEWAY_PROXY_TARGET` uses HTTPS with a private CA, add that CA to the trust store of the `coderd` pods before changing the proxy target.
For the Coder Helm chart, you can mount a CA bundle with `coder.certs.secrets`.
This trust configuration is separate from `aigateway.coderTLS.caSecret`, which configures the connection from the standalone gateway to `coderd`.

## Scale replicas

Run multiple replicas behind a Service or load balancer.
In the provided Helm chart, set the following:

```yaml
coder:
  replicaCount: 3
```

The Service balances requests across ready replicas.
Each replica maintains its own control connection, local caches, concurrency limits, request metrics, logs, and traces.
All replicas fetch provider configuration from the same Coder deployment and write AI session records through `coderd`.
Cost control metrics are registered only on `coderd`, so `coder_ai_gateway_cost_control_*` series never appear on a replica's Prometheus listener.
Budget enforcement also runs in `coderd` over the control connection rather than per replica.
Sticky load balancing can improve cache efficiency, but it is not required for correctness.

When you enable [API dumps](./setup.md#api-dumps), each replica writes dumps to its own local disk.
Use persistent storage if you need those files to survive pod replacement.

The chart requests 1&nbsp;CPU and 1&nbsp;GiB of memory per replica and does not set resource limits.
Measure production traffic before changing requests, limits, or `CODER_AI_GATEWAY_MAX_CONCURRENCY`.
The chart does not create a Horizontal Pod Autoscaler or PodDisruptionBudget.

## Migrate from the embedded gateway

Use a gradual cutover so that you can validate the standalone data plane before moving production traffic:

1. Upgrade `coderd` to the target Coder version.
1. Keep `CODER_AI_GATEWAY_ENABLED=true` on `coderd`.
1. Create a dedicated gateway key.
1. Deploy one standalone replica without changing client or proxy routing.
1. Verify `/readyz`, the key heartbeat, metrics, logs, and traces.
1. Send a test request directly to the standalone gateway and confirm that the AI session appears in Coder.
1. Set `CODER_AI_GATEWAY_PROXY_TARGET` to the standalone endpoint.
1. Update direct client base URLs that should use the standalone endpoint.
1. Scale the standalone deployment after the canary path is stable.

Keep `CODER_AI_GATEWAY_ENABLED=true` on `coderd` after the cutover.
The setting is required for standalone control connections and Coder features that use the in-process gateway.

## Roll back to the embedded gateway

To return traffic to the embedded gateway:

1. Remove `CODER_AI_GATEWAY_PROXY_TARGET`, or set it to `<Coder access URL>/api/v2/ai-gateway`.
1. Restart or upgrade `coderd` so the proxy target change takes effect.
1. Restore direct client base URLs to `<Coder access URL>/api/v2/ai-gateway/<provider-name>/`.
1. Verify requests and AI session records through the embedded route.
1. Scale down or uninstall the standalone deployment.
1. Delete the standalone gateway key after all replicas have stopped.

Delete the key with the following command:

```sh
coder ai-gateway keys delete standalone-production
```

Deleting a key prevents new connections immediately.
An established connection closes when its next heartbeat detects the deletion, within 60 seconds.
The replica then tries to reconnect, receives HTTP 401, and treats that as fatal: the process exits non-zero rather than retrying.
Stop the deployment before deleting its key.

## Version compatibility

The standalone gateway image does not need to match the `coderd` image version exactly.
The components can connect when their AI Gateway API versions are compatible, and `coderd` checks AI Gateway API compatibility whenever a gateway replica connects.

A replica may use the same AI Gateway API version as `coderd` or an earlier minor version of the same major version, but never a newer one.
The gateway treats certain handshake failures from `/api/v2/ai-gateway/serve` as fatal and exits rather than retrying:

- HTTP 400: incompatible API version.
- HTTP 401: invalid gateway key.
- HTTP 403: missing entitlement.
- HTTP 404: endpoint not found.
  `coderd` may be too old and not expose the `/serve` endpoint.

Sequence changes that move both components:

- To upgrade, upgrade `coderd` first, then roll out the standalone gateway release.
- To roll back, roll back the standalone gateway first, then roll back `coderd`.

Running the same Coder release on `coderd` and every standalone replica is the simplest way to stay compatible, but it is not required.
