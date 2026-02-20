# nsjail on ECS

This page describes the runtime and permission requirements for running
Boundary with the **nsjail** jail type on **Amazon ECS**.

## Runtime & Permission Requirements for Running Boundary in ECS

Requirements depend on the node OS and how ECS runs your tasks. The following
examples use **ECS with Self Managed Node Groups** (EC2 launch type).

---

### Example 1: ECS + Self Managed Node Groups + Amazon Linux

On **Amazon Linux** nodes with ECS, the default Docker seccomp profile enforced
by ECS blocks the syscalls needed for Boundary. You must grant `SYS_ADMIN` so that Boundary
can create namespaces and run nsjail.

**Task definition (Terraform) — `linuxParameters`:**

```hcl
container_definitions = jsonencode([{
  name      = "coder-agent"
  image     = "your-coder-agent-image"

  linuxParameters = {
    capabilities = {
      add = ["NET_ADMIN", "SYS_ADMIN"]
    }
  }
}])
```

This gives the container the capabilities required for nsjail when ECS uses the
default Docker seccomp profile.
