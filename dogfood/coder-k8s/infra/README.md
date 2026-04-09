# Infrastructure for coder-k8s template

These files are intended for the **coder/dogfood** repository under
`clusters/dogfood-v2/coder/`. They set up the provisioner, namespace,
and RBAC that the `coder-k8s` template depends on.

## Files

| File                                                      | Target path in `coder/dogfood`                                                                  | Purpose                                              |
|-----------------------------------------------------------|-------------------------------------------------------------------------------------------------|------------------------------------------------------|
| `namespace-coder-workspaces.yaml`                         | `clusters/dogfood-v2/coder/namespace/namespace-coder-workspaces.yaml`                           | Namespace, ResourceQuota, Role, RoleBinding          |
| `values-tagged-k8s.yaml`                                  | `clusters/dogfood-v2/coder/provisioner/configs/values-tagged-k8s.yaml`                          | Helm values for the K8s-tagged provisioner           |
| `values-tagged-k8s-prebuilds.yaml`                        | `clusters/dogfood-v2/coder/provisioner/configs/values-tagged-k8s-prebuilds.yaml`                | Helm values for the K8s-tagged prebuilds provisioner |
| `helmrelease-coder-provisioner-tagged-k8s.yaml`           | `clusters/dogfood-v2/coder/provisioner/helmrelease-coder-provisioner-tagged-k8s.yaml`           | Flux HelmRelease                                     |
| `helmrelease-coder-provisioner-tagged-k8s-prebuilds.yaml` | `clusters/dogfood-v2/coder/provisioner/helmrelease-coder-provisioner-tagged-k8s-prebuilds.yaml` | Flux HelmRelease for prebuilds                       |

## Prerequisites

Before applying these manifests:

1. Create two provisioner keys in the Coder UI at `https://dev.coder.com`:
   - Name: `eks-dogfood-v2-coder-tagged-k8s`, Tags: `cluster=dogfood-v2 env=eks`
   - Name: `eks-dogfood-v2-coder-tagged-k8s-prebuilds`, Tags: `cluster=dogfood-v2 env=eks is_prebuild=true`
2. Add the keys to the SOPS-encrypted `secret-coder-provisioner-keys.yaml`
3. Update the provisioner `kustomization.yaml` to include the new HelmRelease files and ConfigMaps

## Kustomization changes

Add to `clusters/dogfood-v2/coder/provisioner/kustomization.yaml`:

```yaml
resources:
  # ... existing ...
  - helmrelease-coder-provisioner-tagged-k8s.yaml
  - helmrelease-coder-provisioner-tagged-k8s-prebuilds.yaml

configMapGenerator:
  # ... existing ...
  - name: coder-provisioner-values-tagged-k8s
    namespace: coder
    files:
      - values.yaml=configs/values-tagged-k8s.yaml
  - name: coder-provisioner-values-tagged-k8s-prebuilds
    namespace: coder
    files:
      - values.yaml=configs/values-tagged-k8s-prebuilds.yaml
```

## Deploy workflow changes

Add to `.github/workflows/deploy.yaml` in `coder/coder`, in the Flux
reconciliation and rollout restart steps:

```yaml
- flux reconcile helmrelease -n coder coder-provisioner-tagged-k8s
- flux reconcile helmrelease -n coder coder-provisioner-tagged-k8s-prebuilds
- kubectl -n coder rollout restart deployment/coder-provisioner-tagged-k8s
- kubectl -n coder rollout restart deployment/coder-provisioner-tagged-k8s-prebuilds
```
