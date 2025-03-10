# Docker in Workspaces

There are a few ways to run Docker within container-based Coder workspaces.

| Method                                                     | Description                                                                                                                                                        | Limitations                                                                                                                                                                                                                                        |
|------------------------------------------------------------|--------------------------------------------------------------------------------------------------------------------------------------------------------------------|----------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------|
| [Sysbox container runtime](#sysbox-container-runtime)      | Install the Sysbox runtime on your Kubernetes nodes or Docker host(s) for secure docker-in-docker and systemd-in-docker. Works with GKE, EKS, AKS, Docker.         | Requires [compatible nodes](https://github.com/nestybox/sysbox#host-requirements). [Limitations](https://github.com/nestybox/sysbox/blob/master/docs/user-guide/limitations.md)                                                                    |
| [Envbox](#envbox)                                          | A container image with all the packages necessary to run an inner Sysbox container. Removes the need to setup sysbox-runc on your nodes. Works with GKE, EKS, AKS. | Requires running the outer container as privileged (the inner container that acts as the workspace is locked down). Requires compatible [nodes](https://github.com/nestybox/sysbox/blob/master/docs/distro-compat.md#sysbox-distro-compatibility). |
| [Rootless Podman](#rootless-podman)                        | Run Podman inside Coder workspaces. Does not require a custom runtime or privileged containers. Works with GKE, EKS, AKS, RKE, OpenShift                           | Requires smarter-device-manager for FUSE mounts. [See all](https://github.com/containers/podman/blob/main/rootless.md#shortcomings-of-rootless-podman)                                                                                             |
| [Privileged docker sidecar](#privileged-sidecar-container) | Run Docker as a privileged sidecar container.                                                                                                                      | Requires a privileged container. Workspaces can break out to root on the host machine.                                                                                                                                                             |

## Sysbox container runtime

The [Sysbox](https://github.com/nestybox/sysbox) container runtime allows
unprivileged users to run system-level applications, such as Docker, securely
from the workspace containers. Sysbox requires a
[compatible Linux distribution](https://github.com/nestybox/sysbox/blob/master/docs/distro-compat.md)
to implement these security features. Sysbox can also be used to run systemd
inside Coder workspaces. See [Systemd in Docker](#systemd-in-docker).

### Use Sysbox in Docker-based templates

After [installing Sysbox](https://github.com/nestybox/sysbox#installation) on
the Coder host, modify your template to use the sysbox-runc runtime:

```tf
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

After
[installing Sysbox on Kubernetes](https://github.com/nestybox/sysbox/blob/master/docs/user-guide/install-k8s.md),
modify your template to use the sysbox-runc RuntimeClass. This requires the
Kubernetes Terraform provider version 2.16.0 or greater.

```tf
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

## Envbox

[Envbox](https://github.com/coder/envbox) is an image developed and maintained
by Coder that bundles the sysbox runtime. It works by starting an outer
container that manages the various sysbox daemons and spawns an unprivileged
inner container that acts as the user's workspace. The inner container is able
to run system-level software similar to a regular virtual machine (e.g.
`systemd`, `dockerd`, etc). Envbox offers the following benefits over running
sysbox directly on the nodes:

- No custom runtime installation or management on your Kubernetes nodes.
- No limit to the number of pods that run envbox.

Some drawbacks include:

- The outer container must be run as privileged
  - Note: the inner container is _not_ privileged. For more information on the
    security of sysbox containers see sysbox's
    [official documentation](https://github.com/nestybox/sysbox/blob/master/docs/user-guide/security.md).
- Initial workspace startup is slower than running `sysbox-runc` directly on the
  nodes. This is due to `envbox` having to pull the image to its own Docker
  cache on its initial startup. Once the image is cached in `envbox`, startup
  performance is similar.

Envbox requires the same kernel requirements as running sysbox directly on the
nodes. Refer to sysbox's
[compatibility matrix](https://github.com/nestybox/sysbox/blob/master/docs/distro-compat.md#sysbox-distro-compatibility)
to ensure your nodes are compliant.

To get started with `envbox` check out the
[starter template](https://github.com/coder/coder/tree/main/examples/templates/kubernetes-envbox)
or visit the [repo](https://github.com/coder/envbox).

### Authenticating with a Private Registry

Authenticating with a private container registry can be done by referencing the
credentials via the `CODER_IMAGE_PULL_SECRET` environment variable. It is
encouraged to populate this
[environment variable](https://kubernetes.io/docs/tasks/inject-data-application/distribute-credentials-secure/#define-container-environment-variables-using-secret-data)
by using a Kubernetes
[secret](https://kubernetes.io/docs/tasks/configure-pod-container/pull-image-private-registry/#registry-secret-existing-credentials).

Refer to your container registry documentation to understand how to best create
this secret.

The following shows a minimal example using a the JSON API key from a GCP
service account to pull a private image:

```bash
# Create the secret
$ kubectl create secret docker-registry <name> \
  --docker-server=us.gcr.io \
  --docker-username=_json_key \
  --docker-password="$(cat ./json-key-file.yaml)" \
  --docker-email=<service-account-email>
```

```tf
env {
  name = "CODER_IMAGE_PULL_SECRET"
  value_from {
    secret_key_ref {
      name = "<name>"
      key = ".dockerconfigjson"
    }
  }
}
```

## Rootless podman

[Podman](https://docs.podman.io/en/latest/) is Docker alternative that is
compatible with OCI containers specification. which can run rootless inside
Kubernetes pods. No custom RuntimeClass is required.

Before using Podman, please review the following documentation:

- [Basic setup and use of Podman in a rootless environment](https://github.com/containers/podman/blob/main/docs/tutorials/rootless_tutorial.md)

- [Shortcomings of Rootless Podman](https://github.com/containers/podman/blob/main/rootless.md#shortcomings-of-rootless-podman)

1. Enable
   [smart-device-manager](https://gitlab.com/arm-research/smarter/smarter-device-manager#enabling-access)
   to securely expose a FUSE devices to pods.

   ```shell
   cat <<EOF | kubectl create -f -
   apiVersion: apps/v1
   kind: DaemonSet
   metadata:
     name: fuse-device-plugin-daemonset
     namespace: kube-system
   spec:
     selector:
       matchLabels:
         name: fuse-device-plugin-ds
     template:
       metadata:
         labels:
           name: fuse-device-plugin-ds
       spec:
         hostNetwork: true
         containers:
         - name: fuse-device-plugin-ctr
           image: soolaugust/fuse-device-plugin:v1.0
           securityContext:
             allowPrivilegeEscalation: false
             capabilities:
               drop: ["ALL"]
           volumeMounts:
           - name: device-plugin
             mountPath: /var/lib/kubelet/device-plugins
         volumes:
         - name: device-plugin
           hostPath:
             path: /var/lib/kubelet/device-plugins
         imagePullSecrets:
         - name: registry-secret
   EOF
   ```

2. Be sure to label your nodes to enable smarter-device-manager:

   ```shell
   kubectl get nodes
   kubectl label nodes --all smarter-device-manager=enabled
   ```

   > ⚠️ **Warning**: If you are using a managed Kubernetes distribution (e.g.
   > AKS, EKS, GKE), be sure to set node labels via your cloud provider.
   > Otherwise, your nodes may drop the labels and break podman functionality.

3. For systems running SELinux (typically Fedora-, CentOS-, and Red Hat-based
   systems), you might need to disable SELinux or set it to permissive mode.

4. Use this
   [kubernetes-with-podman](https://github.com/coder/community-templates/tree/main/kubernetes-podman)
   example template, or make your own.

   ```shell
   echo "kubernetes-with-podman" | coder templates init
   cd ./kubernetes-with-podman
   coder templates create
   ```

   > For more information around the requirements of rootless podman pods, see:
   > [How to run Podman inside of Kubernetes](https://www.redhat.com/sysadmin/podman-inside-kubernetes)

## Privileged sidecar container

A
[privileged container](https://docs.docker.com/engine/containers/run/#runtime-privilege-and-linux-capabilities)
can be added to your templates to add docker support. This may come in handy if
your nodes cannot run Sysbox.

> [!WARNING]
> This is insecure. Workspaces will be able to gain root access to the host machine.

### Use a privileged sidecar container in Docker-based templates

```tf
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

```tf
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
        run_as_user = 0
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

Additionally, [Sysbox](https://github.com/nestybox/sysbox) can be used to give
workspaces full `systemd` capabilities.

After
[installing Sysbox on Kubernetes](https://github.com/nestybox/sysbox/blob/master/docs/user-guide/install-k8s.md),
modify your template to use the sysbox-runc RuntimeClass. This requires the
Kubernetes Terraform provider version 2.16.0 or greater.

```tf
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
