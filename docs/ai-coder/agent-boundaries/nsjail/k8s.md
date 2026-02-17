# nsjail on Kubernetes

This page describes the runtime and permission requirements for running Agent
Boundaries with the **nsjail** jail type on **Kubernetes**.

## Runtime & Permission Requirements for Running Boundary in Kubernetes

Requirements depend on the node OS and the container runtime. The following
examples use **EKS with Managed Node Groups** for two common node AMIs.

---

### Example 1: EKS + Managed Node Groups + Amazon Linux

On **Amazon Linux** nodes, the default seccomp and runtime behavior typically
allow the syscalls needed for Boundary. You only need to
grant `NET_ADMIN`.

**Container `securityContext`:**

```yaml
apiVersion: v1
kind: Pod
metadata:
  name: coder-agent
spec:
  containers:
    - name: coder-agent
      image: your-coder-agent-image
      securityContext:
        capabilities:
          add:
            - NET_ADMIN
      # ... rest of container spec
```

---

### Example 2: EKS + Managed Node Groups + Bottlerocket

On **Bottlerocket** nodes, the default seccomp profile often blocks the `clone`
syscalls required for unprivileged user namespaces. You must either disable or
modify seccomp for the pod (see [Docker Seccomp Profile Considerations](../docker.md#docker-seccomp-profile-considerations)) or grant `SYS_ADMIN`.

**Option A: `NET_ADMIN` + disable seccomp**

Disabling the seccomp profile allows the container to create namespaces
without granting `SYS_ADMIN` capabilities.

```yaml
apiVersion: v1
kind: Pod
metadata:
  name: coder-agent
spec:
  containers:
    - name: coder-agent
      image: your-coder-agent-image
      securityContext:
        capabilities:
          add:
            - NET_ADMIN
        seccompProfile:
          type: Unconfined
      # ... rest of container spec
```

**Option B: `NET_ADMIN` + `SYS_ADMIN`**

Granting `SYS_ADMIN` bypasses many seccomp restrictions and allows namespace
creation.

```yaml
apiVersion: v1
kind: Pod
metadata:
  name: coder-agent
spec:
  containers:
    - name: coder-agent
      image: your-coder-agent-image
      securityContext:
        capabilities:
          add:
            - NET_ADMIN
            - SYS_ADMIN
      # ... rest of container spec
```

**User namespaces on Bottlerocket**

User namespaces are often disabled (`user.max_user_namespaces=0`) on Bottlerocket
nodes. Check and enable user namespaces:

```bash
# Check current value
sysctl user.max_user_namespaces

# If it returns 0, enable user namespaces
sysctl -w user.max_user_namespaces=65536
```

If `sysctl -w` is not allowed, configure it via Bottlerocket bootstrap settings
when creating the node group (e.g., in Terraform):

```hcl
bootstrap_extra_args = <<-EOT
  [settings.kernel.sysctl]
  "user.max_user_namespaces" = "65536"
EOT
```

This ensures Boundary can create user namespaces with nsjail.
