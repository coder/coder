There are a few ways to run Docker within container-based Coder workspaces.

## Sysbox runtime (recommended)

The [Sysbox](https://github.com/nestybox/sysbox) container runtime allows unprivileged users to run system-level applications, such as Docker, securely from the workspace containers. Sysbox requires a [compatible Linux distribution](https://github.com/nestybox/sysbox/blob/master/docs/distro-compat.md) to implement these security features.

> Sysbox can also be used to run systemd inside Coder workspaces. See [Systemd in Docker](#systemd-in-docker).

### Use Sysbox in Docker-based templates

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

### Use Sysbox in Kubernetes-based templates

After [installing Sysbox on Kubernetes](https://github.com/nestybox/sysbox/blob/master/docs/user-guide/install-k8s.md), modify your template to use the sysbox-runc RuntimeClass. This requires the Kubernetes Terraform provider version 2.16.0 or greater.

```hcl
terraform {
  required_providers {
    coder = {
      source  = "coder/coder"
    }
    kubernetes = {
      source = "hashicorp/kubernetes"
      version = "2.16.0"
    }
  }
}

variable "workspaces_namespace" {
  default = "coder-namespace"
}

data "coder_workspace" "me" {}

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

resource "kubernetes_pod" "dev" {
  count = data.coder_workspace.me.start_count
  metadata {
    name      = "coder-${data.coder_workspace.me.owner}-${data.coder_workspace.me.name}"
    namespace = var.workspaces_namespace
    annotations = {
      "io.kubernetes.cri-o.userns-mode" = "auto:size=65536"
    }
  }

  spec {
  runtime_class_name = "sysbox-runc"
  # Use the Sysbox container runtime (required)
    security_context {
      run_as_user = 1000
      fs_group    = 1000
    }
    container {
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

## Privileged sidecar container

While less secure, you can attach a [privileged container](https://docs.docker.com/engine/reference/run/#runtime-privilege-and-linux-capabilities) to your templates. This may come in handy if your nodes cannot run Sysbox.

### Use a privileged sidecar container in Docker-based templates

```hcl
resource "coder_agent" "main" {
  os             = "linux"
  arch           = "amd64"
}

resource "docker_network" "private_network" {
  name = "network-${data.coder_workspace.me.id}"
}

resource "docker_container" "dind" {
  image      = "docker:dind"
  privileged = true
  name       = "dind-${data.coder_workspace.me.id}"
  entrypoint = ["dockerd", "-H", "tcp://0.0.0.0:2375"]
  networks_advanced {
    name = docker_network.private_network.name
  }
}

resource "docker_container" "workspace" {
  count   = data.coder_workspace.me.start_count
  image   = "codercom/enterprise-base:ubuntu"
  name    = "dev-${data.coder_workspace.me.id}"
  command = ["sh", "-c", coder_agent.main.init_script]
  env = [
    "CODER_AGENT_TOKEN=${coder_agent.main.token}",
    "DOCKER_HOST=${docker_container.dind.name}:2375"
  ]
  networks_advanced {
    name = docker_network.private_network.name
  }
}
```

### Use a privileged sidecar container in Kubernetes-based templates

```hcl
terraform {
  required_providers {
    coder = {
      source  = "coder/coder"
    }
    kubernetes = {
      source = "hashicorp/kubernetes"
      version = "2.16.0"
    }
  }
}

variable "workspaces_namespace" {
  default = "coder-namespace"
}

data "coder_workspace" "me" {}

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

## Systemd in Docker

Additionally, [Sysbox](https://github.com/nestybox/sysbox) can be used to give workspaces full `systemd` capabilities.

### Use systemd in Docker-based templates

After [installing Sysbox](https://github.com/nestybox/sysbox#installation) on the Coder host, modify your template to use the sysbox-runc runtime and start systemd:

```hcl
resource "docker_container" "workspace" {
  image = "codercom/enterprise-base:ubuntu"
  name = "coder-${data.coder_workspace.me.owner}-${lower(data.coder_workspace.me.name)}"

  # Use Sysbox container runtime (required)
  runtime = "sysbox-runc"
  # Run as root in order to start systemd (required)
  user    = "0:0"

  # Start systemd and the Coder agent
  command = ["sh", "-c", <<EOF
    # Start the Coder agent as the "coder" user
    # once systemd has started up
    sudo -u coder --preserve-env=CODER_AGENT_TOKEN /bin/bash -- <<-'    EOT' &
    while [[ ! $(systemctl is-system-running) =~ ^(running|degraded) ]]
    do
      echo "Waiting for system to start... $(systemctl is-system-running)"
      sleep 2
    done
    ${coder_agent.main.init_script}
    EOT

    exec /sbin/init
    EOF
    ,
  ]
  env     = ["CODER_AGENT_TOKEN=${coder_agent.main.token}"]
}

resource "coder_agent" "main" {
  arch = data.coder_provisioner.me.arch
  os   = "linux"
}
```

### Use systemd in Kubernetes-based templates

After [installing Sysbox on Kubernetes](https://github.com/nestybox/sysbox/blob/master/docs/user-guide/install-k8s.md),
modify your template to use the sysbox-runc RuntimeClass. This requires the Kubernetes Terraform provider version 2.16.0 or greater.

```hcl
terraform {
  required_providers {
    coder = {
      source  = "coder/coder"
    }
    kubernetes = {
      source = "hashicorp/kubernetes"
      version = "2.16.0"
    }
  }
}

variable "workspaces_namespace" {
  default = "coder-namespace"
}

data "coder_workspace" "me" {}

resource "coder_agent" "main" {
  os   = "linux"
  arch = "amd64"
  dir  = "/home/coder"
}

resource "kubernetes_pod" "dev" {
  count = data.coder_workspace.me.start_count
  metadata {
    name      = "coder-${data.coder_workspace.me.owner}-${data.coder_workspace.me.name}"
    namespace = var.workspaces_namespace
    annotations = {
      "io.kubernetes.cri-o.userns-mode" = "auto:size=65536"
    }
  }

  spec {

    # Use Sysbox container runtime (required)
    runtime_class_name = "sysbox-runc"

    # Run as root in order to start systemd (required)
    security_context {
      run_as_user = 0
      fs_group    = 0
    }

    container {
      name = "dev"
      env {
        name  = "CODER_AGENT_TOKEN"
        value = coder_agent.main.token
      }
      image = "codercom/enterprise-base:ubuntu"
      command = ["sh", "-c", <<EOF
    # Start the Coder agent as the "coder" user
    # once systemd has started up
    sudo -u coder --preserve-env=CODER_AGENT_TOKEN /bin/bash -- <<-'    EOT' &
    while [[ ! $(systemctl is-system-running) =~ ^(running|degraded) ]]
    do
      echo "Waiting for system to start... $(systemctl is-system-running)"
      sleep 2
    done
    ${coder_agent.main.init_script}
    EOT

    exec /sbin/init
    EOF
      ]
    }
  }
}
```
