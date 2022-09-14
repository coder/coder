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

> Sysbox CE (Community Edition) supports a maximum of 16 pods (workspaces) per node on Kubernetes. See the [Sysbox documentation](https://github.com/nestybox/sysbox/blob/master/docs/user-guide/install-k8s.md#limitations) for more details.

## Privileged sidecar container (Kubernetes)

While less secure, you can attach a [privileged container](https://docs.docker.com/engine/reference/run/#runtime-privilege-and-linux-capabilities) to your templates. This may come in handy if your nodes cannot run Sysbox.

```hcl
resource "coder_agent" "main" {
  os             = "linux"
  arch           = "amd64"
}

resource "kubernetes_pod" "main" {
  count = data.coder_workspace.me.start_count
  metadata {
    name      = "coder-${data.coder_workspace.me.owner}-${data.coder_workspace.me.name}"
    namespace = var.namespace
  }
  spec {
    # Run a privileged dind (Docker in Docker) container
    container {
      name  = "docker-sidecar"
      image = "docker:dind"
      security_context {
        privileged = true
      }
      command = ["dockerd", "-H", "tcp://127.0.0.1:2375"]
    }
    container {
      name    = "dev"
      image   = "codercom/enterprise-base:ubuntu"
      command = ["sh", "-c", coder_agent.main.init_script]
      security_context {
        run_as_user = "1000"
      }
      env {
        name  = "CODER_AGENT_TOKEN"
        value = coder_agent.main.token
      }
      # Use the Docker daemon in the "docker-sidecar" container
      env {
        name  = "DOCKER_HOST"
        value = "localhost:2375"
      }
    }
  }
}

```

## Shared Docker socket (Docker)

While less secure, Docker-based templates can share the host's Docker socket.

````hcl
resource "coder_agent" "main" {
  arch           = data.coder_provisioner.me.arch
  os             = "linux"
  startup_script = <<EOF
    #!/bin/sh

    # Give the internal "coder" user permission
    # to use the Docker socket
    sudo chmod 666 /var/run/socker.sock

    EOF
}

resource "docker_container" "workspace" {
  count   = data.coder_workspace.me.start_count
  image   = "codercom/enterprise-base:ubuntu"
  name    = "coder-${data.coder_workspace.me.owner}-${lower(data.coder_workspace.me.name)}"
  command = ["sh", "-c", coder_agent.main.init_script]
  env     = ["CODER_AGENT_TOKEN=${coder_agent.main.token}"]
  volumes {
    container_path = "/var/run/docker.sock"
    host_path      = "/var/run/docker.sock"
  }
}
```hcl
````
