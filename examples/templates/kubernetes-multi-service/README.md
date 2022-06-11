---
name: Develop multiple services in Kubernetes
description: Get started with Kubernetes development.
tags: [cloud, kubernetes]
---

# Getting started

## RBAC

The Coder provisioner requires permission to administer pods to use this template.  The template
creates workspaces in a single Kubernetes namespace, using the `workspaces_namespace` parameter set
while creating the template.

Create a role as follows and bind it to the user or service account that runs the coder host.

```yaml
apiVersion: rbac.authorization.k8s.io/v1
kind: Role
metadata:
  name: coder
rules:
- apiGroups: [""]
  resources: ["pods"]
  verbs: ["*"]
```

## Authentication

This template can authenticate using in-cluster authentication, or using a kubeconfig local to the
Coder host.  For additional authentication options, consult the [Kubernetes provider
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
authenticate to Kubernetes.  Be sure to bind a [role with appropriate permission](#rbac) to the
service account.  For example, assuming the Coder host runs in the same namespace as you intend
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

