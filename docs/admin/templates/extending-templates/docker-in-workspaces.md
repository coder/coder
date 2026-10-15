---
title: Docker in workspaces
---

There are a few ways to run Docker within container-based Coder workspaces.

| Method                                                     | Description                                                                                                                                                        | Limitations                                                                                                                                                                                                                                        |
|------------------------------------------------------------|--------------------------------------------------------------------------------------------------------------------------------------------------------------------|----------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------|
| [Sysbox container runtime](#sysbox-container-runtime)      | Install the Sysbox runtime on your Kubernetes nodes or Docker host(s) for secure docker-in-docker and systemd-in-docker. Works with GKE, EKS, AKS, Docker.         | Requires [compatible nodes](https://github.com/nestybox/sysbox#host-requirements). [Limitations](https://github.com/nestybox/sysbox/blob/master/docs/user-guide/limitations.md)                                                                    |
| [Envbox](#envbox)                                          | A container image with all the packages necessary to run an inner Sysbox container. Removes the need to setup sysbox-runc on your nodes. Works with GKE, EKS, AKS. | Requires running the outer container as privileged (the inner container that acts as the workspace is locked down). Requires compatible [nodes](https://github.com/nestybox/sysbox/blob/master/docs/distro-compat.md#sysbox-distro-compatibility). |
| [Rootless Podman](#rootless-podman)                        | Run Podman inside Coder workspaces. Does not require a custom runtime or privileged containers. Works with GKE, EKS, AKS, RKE, OpenShift                           | Requires a FUSE device plugin, or native overlay storage on kernel 5.13 or later. [See all](https://github.com/containers/podman/blob/main/rootless.md#shortcomings-of-rootless-podman)                                                            |
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
  image   = "codercom/example-base:ubuntu"
  env     = ["CODER_AGENT_TOKEN=${coder_agent.main.token}"]
  command = ["sh", "-c", coder_agent.main.init_script]
  # Use the Sysbox container runtime (required)
  runtime = "sysbox-runc"
}

resource "coder_agent" "main" {
  arch           = data.coder_provisioner.me.arch
  os             = "linux"
  startup_script = <<-EOF
    #!/bin/sh
    set -e
    sudo service docker start
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
  startup_script = <<-EOF
    #!/bin/sh
    set -e
    sudo service docker start
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
      image = "codercom/example-base:ubuntu"
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
[starter template](../../../../examples/templates/kubernetes-envbox)
or visit the [repo](https://github.com/coder/envbox).

<a id="authenticating-with-a-private-registry"></a>

### Authenticate with a private registry

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

```sh
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
   [smart-device-manager](https://github.com/smarter-project/smarter-device-manager#enabling-access)
   to securely expose a FUSE devices to pods.

   ```sh
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

   ```sh
   kubectl get nodes
   kubectl label nodes --all smarter-device-manager=enabled
   ```

   > ⚠️ **Warning**: If you are using a managed Kubernetes distribution (e.g.
   > AKS, EKS, GKE), be sure to set node labels via your cloud provider.
   > Otherwise, your nodes may drop the labels and break podman functionality.

3. For systems running SELinux (typically Fedora-, CentOS-, and Red Hat-based
   systems), you might need to disable SELinux or set it to permissive mode.

4. Create a template from the built-in `kubernetes` starter, then adapt it for rootless Podman:

   ```sh
   coder templates init --id kubernetes ./kubernetes-podman
   cd ./kubernetes-podman
   ```

   The `kubernetes` starter already runs the workspace pod as a non-root user (`run_as_user = 1000`, `fs_group = 1000`, `run_as_non_root = true`), which rootless Podman requires. In the generated `main.tf`, apply the Podman changes to the `kubernetes_deployment_v1.main` pod template (marked `# Podman` in the following snippet):

   ```tf
   spec {
     template {
       metadata {
         # ...
         # Podman: allow Podman to create nested containers. The annotation
         # key must match the container name below (`dev`).
         annotations = {
           "container.apparmor.security.beta.kubernetes.io/dev" = "unconfined"
         }
       }
       spec {
         # The starter already sets these; rootless Podman requires them.
         security_context {
           run_as_user     = 1000
           fs_group        = 1000
           run_as_non_root = true
         }

         container {
           name = "dev"
           # Podman: base image with Podman and fuse-overlayfs preinstalled.
           image = "codercom/example-podman:ubuntu"
           # ...
           resources {
             limits = {
               # ...
               # Podman: FUSE device exposed by smarter-device-manager (step 1).
               "github.com/fuse" = "1"
             }
           }
         }
       }
     }
   }
   ```

   The `codercom/example-podman` image is built and maintained in
   [`coder/images`](https://github.com/coder/images).

   > On Kubernetes 1.30+, the
   > `container.apparmor.security.beta.kubernetes.io/<container>` annotation is
   > deprecated in favor of the `securityContext.appArmorProfile` field. The
   > annotation still works and remains the most broadly compatible option, so
   > switch to the field once your cluster and Terraform Kubernetes provider
   > both support it.

   Push the template to your deployment:

   ```sh
   coder templates push
   ```

   > For more information around the requirements of rootless podman pods, see:
   > [How to run Podman inside of Kubernetes](https://www.redhat.com/sysadmin/podman-inside-kubernetes)

### Rootless Podman on Bottlerocket nodes

Rootless containers rely on Linux user-namespaces.
[Bottlerocket](https://github.com/bottlerocket-os/bottlerocket) disables them by default (`user.max_user_namespaces = 0`), so Podman commands will return an error until you raise the limit:

```txt
cannot clone: Invalid argument
user namespaces are not enabled in /proc/sys/user/max_user_namespaces
```

1. Add a `user.max_user_namespaces` value to your Bottlerocket user data to use rootless Podman on the node:

  ```toml
  [settings.kernel.sysctl]
  "user.max_user_namespaces" = "65536"
  ```

1. Reboot the node.
1. Verify that the value is more than `0`:

  ```sh
  sysctl -n user.max_user_namespaces
  ```

For Karpenter-managed Bottlerocket nodes, add the `user.max_user_namespaces` setting in your `EC2NodeClass`:

```yaml
apiVersion: karpenter.k8s.aws/v1
kind: EC2NodeClass
metadata:
  name: bottlerocket-rootless
spec:
  amiFamily: Bottlerocket # required for BR-style userData
  # …
  userData: |
    [settings.kernel]
    sysctl = { "user.max_user_namespaces" = "65536" }
```

### Rootless Podman on EKS Auto Mode

The `EC2NodeClass` above applies to self-managed Karpenter nodes.
On EKS Auto Mode, raise `user.max_user_namespaces` with the `advancedCompute` field on a custom `NodeClass` (`eks.amazonaws.com`).
Auto Mode `NodeClass` resources [support `advancedCompute`](https://aws.amazon.com/about-aws/whats-new/2026/10/eks-auto-mode-advanced-compute-config/), so this works on pure Auto Mode nodes with no managed node group.

```yaml
apiVersion: eks.amazonaws.com/v1
kind: NodeClass
metadata:
  name: podman
spec:
  # Reuse the Auto Mode node role so the EKS access entry already exists.
  role: <your Auto Mode node role>
  subnetSelectorTerms:
    - id: <subnet ID 1>
    - id: <subnet ID 2>
  securityGroupSelectorTerms:
    - id: <cluster security group ID>
  advancedCompute:
    kernel:
      sysctl:
        user.max_user_namespaces: 65536
```

Reference the `NodeClass` from a `NodePool` and schedule Podman workspaces onto that pool with a node selector.
Refer to [Create a Node Class for Amazon EKS](https://docs.aws.amazon.com/eks/latest/userguide/create-node-class.html) for the full `advancedCompute` reference.

If you can't use `advancedCompute`, run Podman workspaces on a managed node group instead.
The default AL2023 AMIs ship with user namespaces enabled.

Pods that set `hostUsers: false` also need `advancedCompute.kubelet.idsPerPod` raised above the 65536 default before rootless Podman can allocate subordinate ID ranges inside them.
The value must be a multiple of 65536, and `idsPerPod` requires `user.max_user_namespaces` to be set on the same `NodeClass`:

```yaml
  advancedCompute:
    kernel:
      sysctl:
        user.max_user_namespaces: 65536
    kubelet:
      idsPerPod: 1048576
```

### Use native overlay storage instead of fuse-overlayfs

Kernel overlayfs cannot stack on the workspace container's own overlay root filesystem.
Without extra configuration, Podman falls back to `fuse-overlayfs`, which requires the `/dev/fuse` device and the smarter-device-manager setup above.
On nodes with kernel 5.13 or later, you can skip the FUSE device entirely:

1. Put the Podman graph root on a real filesystem such as ext4 or xfs.
   If your template mounts a persistent volume at `/home/coder`, like the `kubernetes` starter, the default graph root already qualifies and keeps images across workspace restarts.
   For ephemeral workspaces, mount an `emptyDir` volume at `/home/coder/.local/share/containers` instead; pulled images and build cache are lost when the workspace stops.
1. Clear the `mount_program` setting, because some Podman images pin it to `fuse-overlayfs` in `/etc/containers/storage.conf`.
   Write the override from the agent `startup_script`, which can also start the Docker-compatible API socket:

   ```tf
   startup_script = <<-EOT
     mkdir -p ~/.config/containers
     printf '[storage]\ndriver = "overlay"\n\n[storage.options.overlay]\nmount_program = ""\n' > ~/.config/containers/storage.conf
     (podman system service --time=0 unix:///tmp/podman.sock >/dev/null 2>&1 &)
     for i in $(seq 50); do [ -S /tmp/podman.sock ] && break; sleep 0.1; done
   EOT
   ```

   Set `DOCKER_HOST=unix:///tmp/podman.sock` for Docker clients that expect a daemon socket.
   Also set `DOCKER_BUILDKIT=0` so that `docker build` and `docker compose build` use the classic builder, because BuildKit can't start inside the workspace.
   `docker buildx` commands still require BuildKit and aren't supported.

With both settings, Podman uses native kernel overlay, which performs better than `fuse-overlayfs` and removes the device plugin dependency.
When you use native overlay, skip the smarter-device-manager steps above and remove the `github.com/fuse` limit from the pod spec; without the device plugin, that limit keeps pods in `Pending`.

A complete reference template is available in [`examples/templates/kubernetes-podman-rootless`](https://github.com/coder/coder/tree/main/examples/templates/kubernetes-podman-rootless).

### Nested container engines

Rootless Podman supports one level of nesting: the workspace runs containers, builds images, and serves the Docker-compatible API socket.
Running another container engine inside a container that the workspace engine started is not supported.
This includes the dev container [`docker-in-docker` feature](https://github.com/devcontainers/features/tree/main/src/docker-in-docker), which starts a nested daemon inside the dev container.

Use the [`docker-outside-of-docker` feature](https://github.com/devcontainers/features/tree/main/src/docker-outside-of-docker) instead.
The dev container drives the workspace engine through its socket, so containers run as siblings of the dev container instead of nested inside it.
Keep the repository path identical inside the dev container and the workspace, for example with `workspaceMount`, so bind mounts resolve against the engine correctly.

The feature mounts the workspace's `/var/run/docker.sock` by default, which doesn't exist with rootless Podman.
Start the API socket from the agent `startup_script` as shown in [Use native overlay storage](#use-native-overlay-storage-instead-of-fuse-overlayfs), then override the mount in `devcontainer.json` to point at it:

```json
"mounts": [
  {
    "source": "/tmp/podman.sock",
    "target": "/var/run/docker-host.sock",
    "type": "bind"
  }
]
```

Building dev container features, such as `docker-outside-of-docker`, fails against rootless Podman because the dev container CLI builds them with BuildKit, which can't start inside the workspace.
Run `devcontainer up --buildkit never` with `DOCKER_BUILDKIT=0` set to use the classic builder instead.
Coder's [dev container integration](../../integrations/devcontainers/index.md) doesn't pass this flag, so it doesn't support rootless Podman workspaces yet.
To run dev containers on Kubernetes without a container engine in the workspace, use [Envbuilder](../../integrations/devcontainers/envbuilder/index.md) instead.

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
  image   = "codercom/example-base:ubuntu"
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
      image   = "codercom/example-base:ubuntu"
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
      image = "codercom/example-base:ubuntu"
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
