---
name: Develop multiple services in Kubernetes
description: Get started with Kubernetes development.
tags: [cloud, kubernetes]
---

# Authentication

This template has several ways to authenticate to a Kubernetes cluster.

## kubeconfig (Coder host)

If the Coder host has a local `~/.kube/config`, this can be used to authenticate with Coder. Make sure this is on the same user running the `coder` service.

## ServiceAccount

Create a ServiceAccount and role on your cluster to authenticate your template with Coder.

1. Run the following command on a device with Kubernetes context:

    ```sh
    CODER_NAMESPACE=default
    kubectl apply -n $CODER_NAMESPACE -f - <<EOF
    apiVersion: v1
    kind: ServiceAccount
    metadata:
    name: coder
    ---
    apiVersion: rbac.authorization.k8s.io/v1
    kind: Role
    metadata:
    name: coder
    rules:
    - apiGroups: ["", "apps", "networking.k8s.io"] # "" indicates the core API group
        resources: ["persistentvolumeclaims", "pods", "deployments", "services", "secrets", "pods/exec","pods/log", "events", "networkpolicies", "serviceaccounts"]
        verbs: ["create", "get", "list", "watch", "update", "patch", "delete", "deletecollection"]
    - apiGroups: ["metrics.k8s.io", "storage.k8s.io"]
        resources: ["pods", "storageclasses"]
        verbs: ["get", "list", "watch"]
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
    EOF
    ```

   1. Use the following commands to fetch the values:

        **Cluster IP:**

        ```sh
        kubectl cluster-info | grep "control plane"
        ```

        **CA certificate**

        ```sh
        kubectl get secrets -n $CODER_NAMESPACE -o jsonpath="{.items[?(@.metadata.annotations['kubernetes\.io/service-account\.name']=='coder')].data['ca\.crt']}{'\n'}"
        ```

        **Token**

        ```sh
        kubectl get secrets -n $CODER_NAMESPACE -o jsonpath="{.items[?(@.metadata.annotations['kubernetes\.io/service-account\.name']=='coder')].data['token']}{'\n'}"
        ```

        **Namespace**

        This should be the same as `$CODER_NAMESPACE`, set in step 1.
