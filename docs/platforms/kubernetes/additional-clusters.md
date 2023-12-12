# Additional clusters

With Coder, you can deploy workspaces in additional Kubernetes clusters using
different
[authentication methods](https://registry.terraform.io/providers/hashicorp/kubernetes/latest/docs#authentication)
in the Terraform provider.

![Region picker in "Create Workspace" screen](../../images/platforms/kubernetes/region-picker.png)

## Option 1) Kubernetes contexts and kubeconfig

First, create a kubeconfig file with
[multiple contexts](https://kubernetes.io/docs/tasks/access-application-cluster/configure-access-multiple-clusters/).

```shell
kubectl config get-contexts

CURRENT   NAME                        CLUSTER
          workspaces-europe-west2-c   workspaces-europe-west2-c
*         workspaces-us-central1-a    workspaces-us-central1-a
```

### Kubernetes control plane

If you deployed Coder on Kubernetes, you can attach a kubeconfig as a secret.

This assumes Coder is deployed on the `coder` namespace and your kubeconfig file
is in ~/.kube/config.

```shell
kubectl create secret generic kubeconfig-secret -n coder --from-file=~/.kube/config
```

Modify your helm values to mount the secret:

```yaml
coder:
  # ...
  volumes:
    - name: "kubeconfig-mount"
      secret:
        secretName: "kubeconfig-secret"
  volumeMounts:
    - name: "kubeconfig-mount"
      mountPath: "/mnt/secrets/kube"
      readOnly: true
```

[Upgrade Coder](../../install/kubernetes.md#upgrading-coder-via-helm) with these
new values.

### VM control plane

If you deployed Coder on a VM, copy the kubeconfig file to
`/home/coder/.kube/config`.

### Create a Coder template

You can start from our
[example template](https://github.com/coder/coder/tree/main/examples/templates/kubernetes).
From there, add [template parameters](../../templates/parameters.md) to allow
developers to pick their desired cluster.

```hcl
# main.tf

data "coder_parameter" "kube_context" {
  name         = "kube_context"
  display_name = "Cluster"
  default      = "workspaces-us-central1-a"
  mutable      = false
  option {
    name  = "US Central"
    icon  = "/emojis/1f33d.png"
    value = "workspaces-us-central1-a"
  }
  option {
    name  = "Europe West"
    icon  = "/emojis/1f482.png"
    value = "workspaces-europe-west2-c"
  }
}

provider "kubernetes" {
  config_path    = "~/.kube/config" # or /mnt/secrets/kube/config for Kubernetes
  config_context = data.coder_parameter.kube_context.value
}
```

## Option 2) Kubernetes ServiceAccounts

Alternatively, you can authenticate with remote clusters with ServiceAccount
tokens. Coder can store these secrets on your behalf with
[managed Terraform variables](../../templates/parameters.md#managed-terraform-variables).

Alternatively, these could also be fetched from Kubernetes secrets or even
[Hashicorp Vault](https://registry.terraform.io/providers/hashicorp/vault/latest/docs/data-sources/generic_secret).

This guide assumes you have a `coder-workspaces` namespace on your remote
cluster. Change the namespace accordingly.

### Create a ServiceAccount

Run this command against your remote cluster to create a ServiceAccount, Role,
RoleBinding, and token:

```shell
kubectl apply -n coder-workspaces -f - <<EOF
apiVersion: v1
kind: ServiceAccount
metadata:
  name: coder-v2
---
apiVersion: v1
kind: Secret
metadata:
  name: coder-v2
  annotations:
    kubernetes.io/service-account.name: coder-v2
type: kubernetes.io/service-account-token
---
apiVersion: rbac.authorization.k8s.io/v1
kind: Role
metadata:
  name: coder-v2
rules:
  - apiGroups: ["", "apps", "networking.k8s.io"]
    resources: ["persistentvolumeclaims", "pods", "deployments", "services", "secrets", "pods/exec","pods/log", "events", "networkpolicies", "serviceaccounts"]
    verbs: ["create", "get", "list", "watch", "update", "patch", "delete", "deletecollection"]
  - apiGroups: ["metrics.k8s.io", "storage.k8s.io"]
    resources: ["pods", "storageclasses"]
    verbs: ["get", "list", "watch"]
---
apiVersion: rbac.authorization.k8s.io/v1
kind: RoleBinding
metadata:
  name: coder-v2
subjects:
  - kind: ServiceAccount
    name: coder-v2
roleRef:
  kind: Role
  name: coder-v2
  apiGroup: rbac.authorization.k8s.io
EOF
```

The output should be similar to:

```text
serviceaccount/coder-v2 created
secret/coder-v2 created
role.rbac.authorization.k8s.io/coder-v2 created
rolebinding.rbac.authorization.k8s.io/coder-v2 created
```

### 2. Modify the Kubernetes template

You can start from our
[example template](https://github.com/coder/coder/tree/main/examples/templates/kubernetes).

```hcl
variable "host" {
  description = "Cluster host address"
  sensitive   = true
}

variable "cluster_ca_certificate" {
  description = "Cluster CA certificate (base64 encoded)"
  sensitive   = true
}

variable "token" {
  description = "Cluster CA token (base64 encoded)"
  sensitive   = true
}

variable "namespace" {
  description = "Namespace"
}

provider "kubernetes" {
  host                   = var.host
  cluster_ca_certificate = base64decode(var.cluster_ca_certificate)
  token                  = base64decode(var.token)
}
```

### Create Coder template with managed variables

Fetch the values from the secret and pass them to Coder. This should work on
macOS and Linux.

To get the cluster address:

```shell
kubectl cluster-info
Kubernetes control plane is running at https://example.domain:6443

export CLUSTER_ADDRESS=https://example.domain:6443
```

To fetch the CA certificate and token:

```shell
export CLUSTER_CA_CERTIFICATE=$(kubectl get secrets coder-v2 -n coder-workspaces -o jsonpath="{.data.ca\.crt}")

export CLUSTER_SERVICEACCOUNT_TOKEN=$(kubectl get secrets coder-v2 -n coder-workspaces -o jsonpath="{.data.token}")
```

Create the template with these values:

```shell
coder templates create \
    --variable host=$CLUSTER_ADDRESS \
    --variable cluster_ca_certificate=$CLUSTER_CA_CERTIFICATE \
    --variable token=$CLUSTER_SERVICEACCOUNT_TOKEN \
    --variable namespace=coder-workspaces
```

If you're on a Windows machine (or if one of the commands fail), try grabbing
the values manually:

```shell
# Get cluster API address
kubectl cluster-info

# Get cluster CA and token (base64 encoded)
kubectl get secrets coder-service-account-token -n coder-workspaces -o jsonpath="{.data}"

coder templates create \
    --variable host=API_ADDRESS \
    --variable cluster_ca_certificate=CLUSTER_CA_CERTIFICATE \
    --variable token=CLUSTER_SERVICEACCOUNT_TOKEN \
    --variable namespace=coder-workspaces
```
