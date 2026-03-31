# Bridge Dump Volume — dogfood-v2

Shared `ReadWriteMany` volume for collecting intercepted LLM request/response
dumps written by `coder/aibridge` via the `BRIDGE_DUMP_DIR` environment
variable.

## Files

| File | Purpose |
|---|---|
| `bridge-dump-pvc.yaml` | `PersistentVolumeClaim` (`ReadWriteMany`, `standard-rwx` storage class) |
| `bridge-dump-values.yaml` | Helm values overlay that mounts the PVC and sets `BRIDGE_DUMP_DIR` |

## How it works

When `BRIDGE_DUMP_DIR` is set, each aibridge provider (Anthropic, OpenAI,
Copilot) writes raw HTTP request/response pairs to disk as they are
intercepted. The directory layout is:

```
/var/run/bridge-dump/
  {provider}/
    {model}/
      {timestamp}-{uuid}.req.txt
      {timestamp}-{uuid}.resp.txt
    passthrough/
      {timestamp}-{urlpath}-{uuid}.req.txt
      {timestamp}-{urlpath}-{uuid}.resp.txt
```

Because the PVC uses `ReadWriteMany`, every coderd replica in the deployment
can write concurrently—dumps land in the same filesystem regardless of which
replica handles the request.

## Deploying

```bash
# 1. Create the PVC (once)
kubectl apply -f bridge-dump-pvc.yaml -n <namespace>

# 2. Upgrade the Helm release with the values overlay
helm upgrade coder coder-v2/coder \
  -n <namespace> \
  -f values.yaml \
  -f bridge-dump-values.yaml
```

## Notes

- `standard-rwx` on GKE is backed by Cloud Filestore (basic tier, 1 TiB
  minimum). If a smaller volume is needed, consider a custom NFS provisioner
  or a different `ReadWriteMany`-capable storage class.
- Sensitive headers (`Authorization`, `X-Api-Key`, etc.) are automatically
  redacted by the aibridge dump middleware.
