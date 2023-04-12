---
name: Develop in Kubernetes
description: Get started with Kubernetes development.
tags: [cloud, kubernetes]
icon: /icon/k8s.png
---

# Getting started

This template creates a pod running the `codercom/enterprise-base:ubuntu` image.

## Authentication

This template can authenticate using in-cluster authentication, or using a kubeconfig local to the
Coder host. For additional authentication options, consult the [Kubernetes provider
documentation](https://registry.terraform.io/providers/hashicorp/kubernetes/latest/docs).

### kubeconfig on Coder host

If the Coder host has a local `~/.kube/config`, you can use this to authenticate
with Coder. Make sure this is done with same user that's running the `coder` service.

To use this authentication, set the parameter `use_kubeconfig` to true.

### In-cluster authentication

If the Coder host runs in a Pod on the same Kubernetes cluster as you are creating workspaces in,
you can use in-cluster authentication.

To use this authentication, set the parameter `use_kubeconfig` to false.

The Terraform provisioner will automatically use the service account associated with the pod to
authenticate to Kubernetes. Be sure to bind a [role with appropriate permission](#rbac) to the
service account. For example, assuming the Coder host runs in the same namespace as you intend
to create workspaces:

```yaml
apiVersion: v1
kind: ServiceAccount
metadata:
  name: coder

---
apiVersion: rbac.authorization.k8s.io/v1
kind: RoleBinding
metadata:
  name: coder
subjects:
  - kind: ServiceAccount
    name: coder
roleRef:
  kind: Role
  name: coder
  apiGroup: rbac.authorization.k8s.io
```

Then start the Coder host with `serviceAccountName: coder` in the pod spec.

### Authenticate against external clusters

You may want to deploy workspaces on a cluster outside of the Coder control plane. Refer to the [Coder docs](https://coder.com/docs/v2/latest/platforms/kubernetes/additional-clusters) to learn how to modify your template to authenticate against external clusters.

## Namespace

The target namespace in which the pod will be deployed is defined via the `coder_workspace`
variable. The namespace must exist prior to creating workspaces.

## Persistence

The `/home/coder` directory in this example is persisted via the attached PersistentVolumeClaim.
Any data saved outside of this directory will be wiped when the workspace stops.

Since most binary installations and environment configurations live outside of
the `/home` directory, we suggest including these in the `startup_script` argument
of the `coder_agent` resource block, which will run each time the workspace starts up.

For example, when installing the `aws` CLI, the install script will place the
`aws` binary in `/usr/local/bin/aws`. To ensure the `aws` CLI is persisted across
workspace starts/stops, include the following code in the `coder_agent` resource
block of your workspace template:

```terraform
resource "coder_agent" "main" {
  startup_script = <<-EOT
    set -e
    # install AWS CLI
    curl "https://awscli.amazonaws.com/awscli-exe-linux-x86_64.zip" -o "awscliv2.zip"
    unzip awscliv2.zip
    sudo ./aws/install
  EOT
}
```

## code-server

`code-server` is installed via the `startup_script` argument in the `coder_agent`
resource block. The `coder_app` resource is defined to access `code-server` through
the dashboard UI over `localhost:13337`.
