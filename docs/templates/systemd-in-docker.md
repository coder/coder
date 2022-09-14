The [Sysbox](https://github.com/nestybox/sysbox) container runtime allows unprivileged users to run system-level applications, such as Systemd, securely from the workspace containers. Sysbox requires a [compatible Linux distribution](https://github.com/nestybox/sysbox/blob/master/docs/distro-compat.md) to implement these security features.

## Docker

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

## Kubernetes

After [installing Sysbox on Kubernetes](https://github.com/nestybox/sysbox/blob/master/docs/user-guide/install-k8s.md), modify your template to use the sysbox-runc RuntimeClass.

> Currently, the official [Kubernetes Terraform Provider](https://registry.terraform.io/providers/hashicorp/kubernetes/latest) does not support specifying a custom RuntimeClass. [mingfang/k8s](https://registry.terraform.io/providers/mingfang/k8s), a third-party provider, can be used instead.

```hcl
terraform {
  required_providers {
    coder = {
      source  = "coder/coder"
    }
    k8s = {
      source = "mingfang/k8s"
    }
  }
}


resource "coder_agent" "main" {
  os   = "linux"
  arch = "amd64"
  dir  = "/home/coder"
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


  spec {

    # Use Sysbox container runtime (required)
    runtime_class_name = "sysbox-runc"

    # Run as root in order to start systemd (required)
    security_context {
      run_asuser = 0
      fsgroup    = 0
    }

    containers {
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
