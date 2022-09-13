I am not really sure what to put here yet.

## Sysbox runtime (recommended)

The Sysbox container runtime allows unprivileged users to run system-level applications, such as Docker, securely from the workspace containers. Sysbox requires a [compatible Linux distribution](https://github.com/nestybox/sysbox/blob/master/docs/distro-compat.md) to implement these security features.

### Use Sysbox in Docker-based templates:

After [installing Sysbox](https://github.com/nestybox/sysbox#installation) on the Coder host, modify your template to use the sysbox-runc runtime:

```hcl
resource "docker_container" "workspace" {
  # ...
  name    = "coder-${data.coder_workspace.me.owner}-${lower(data.coder_workspace.me.name)}"
  image   = "codercom/enterprise-base:ubuntu"
  runtime = "sysbox-runc"
  env     = ["CODER_AGENT_TOKEN=${coder_agent.main.token}"]
  command = ["sh", "-c", coder_agent.main.init_script]
}

resource "coder_agent" "main" {
  arch           = data.coder_provisioner.me.arch
  os             = "linux"
  startup_script = <<EOF
    #!/bin/sh

    # Start docker (you can also use `dockerd&`)
    sudo service docker start

    # ...
    EOF
}
```

> Sysbox can also be used to run systemd inside Coder workspaces. See [Systemd in Docker](./systemd-in-docker.md)

### Use Sysbox in Kubernetes-based templates:

```hcl

```

## Shared Docker socket

## Privileged sidecar container
