There are a few ways to run Docker within container-based Coder workspaces.

## Sysbox runtime (recommended)

The [Sysbox](https://github.com/nestybox/sysbox) container runtime allows unprivileged users to run system-level applications, such as Docker, securely from the workspace containers. Sysbox requires a [compatible Linux distribution](https://github.com/nestybox/sysbox/blob/master/docs/distro-compat.md) to implement these security features.

> Sysbox can also be used to run systemd inside Coder workspaces. See [Systemd in Docker](./systemd-in-docker.md).

### Use Sysbox in Docker-based templates:

After [installing Sysbox](https://github.com/nestybox/sysbox#installation) on the Coder host, modify your template to use the sysbox-runc runtime:

```hcl
resource "docker_container" "workspace" {
  # ...
  name    = "coder-${data.coder_workspace.me.owner}-${lower(data.coder_workspace.me.name)}"
  image   = "codercom/enterprise-base:ubuntu"
  env     = ["CODER_AGENT_TOKEN=${coder_agent.main.token}"]
  command = ["sh", "-c", coder_agent.main.init_script]
  # Use the Sysbox container runtime (required)
  runtime = "sysbox-runc"
}

resource "coder_agent" "main" {
  arch           = data.coder_provisioner.me.arch
  os             = "linux"
  startup_script = <<EOF
    #!/bin/sh

    # Start Docker
    sudo dockerd &

    # ...
    EOF
}
```

### Use Sysbox in Kubernetes-based templates:

After [installing Sysbox on Kubernetes](https://github.com/nestybox/sysbox/blob/master/docs/user-guide/install-k8s.md), modify your template to use the sysbox-runc RuntimeClass.

> Currently, the official [Kubernetes Terraform Provider](https://registry.terraform.io/providers/hashicorp/kubernetes/latest) does not support specifying a custom RuntimeClass. [mingfang/k8s](https://registry.terraform.io/providers/mingfang/k8s), a third-party provider, can be used instead.

```hcl
resource "coder_agent" "main" {
  os   = "linux"
  arch = "amd64"
  dir  = "/home/coder"
  startup_script = <<EOF
    #!/bin/sh

    # Start Docker
    sudo dockerd &

    # ...
  EOF
}

resource "k8s_core_v1_pod" "dev" {
  count = data.coder_workspace.me.start_count
  metadata {
    name      = "coder-${data.coder_workspace.me.owner}-${data.coder_workspace.me.name}"
    namespace = var.workspaces_namespace
    annotations = {
      "io.kubernetes.cri-o.userns-mode" = "auto:size=65536"
    }
  }

  # Use the Sysbox container runtime (required)
  runtime_class_name = "sysbox-runc

  spec {
    security_context {
      run_asuser = 1000
      fsgroup    = 1000
    }
    containers {
      name = "dev"
      env {
        name  = "CODER_AGENT_TOKEN"
        value = coder_agent.main.token
      }
      image = "codercom/enterprise-base:ubuntu"
      command = ["sh", "-c", coder_agent.main.init_script]
    }
  }
}
```

## Privileged sidecar container (Docker and Kubernetes)

TODO

## Shared Docker socket (Docker only)

TODO
