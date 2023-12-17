---
display_name: Envbox (Kubernetes)
description: Provision envbox pods as Coder workspaces
icon: ../../../site/static/icon/k8s.png
maintainer_github: coder
verified: true
tags: [kubernetes, containers, docker-in-docker]
---

# envbox

## Introduction

`envbox` is an image that enables creating non-privileged containers capable of running system-level software (e.g. `dockerd`, `systemd`, etc) in Kubernetes.

It mainly acts as a wrapper for the excellent [sysbox runtime](https://github.com/nestybox/sysbox/) developed by [Nestybox](https://www.nestybox.com/). For more details on the security of `sysbox` containers see sysbox's [official documentation](https://github.com/nestybox/sysbox/blob/master/docs/user-guide/security.md).

## Envbox Configuration

The following environment variables can be used to configure various aspects of the inner and outer container.

| env                        | usage                                                                                                                                                                                                                                                                                                                                                                                                                                                                           | required |
| -------------------------- | ------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------- | -------- |
| `CODER_INNER_IMAGE`        | The image to use for the inner container.                                                                                                                                                                                                                                                                                                                                                                                                                                       | True     |
| `CODER_INNER_USERNAME`     | The username to use for the inner container.                                                                                                                                                                                                                                                                                                                                                                                                                                    | True     |
| `CODER_AGENT_TOKEN`        | The [Coder Agent](https://coder.com/docs/v2/latest/about/architecture#agents) token to pass to the inner container.                                                                                                                                                                                                                                                                                                                                                             | True     |
| `CODER_INNER_ENVS`         | The environment variables to pass to the inner container. A wildcard can be used to match a prefix. Ex: `CODER_INNER_ENVS=KUBERNETES_*,MY_ENV,MY_OTHER_ENV`                                                                                                                                                                                                                                                                                                                     | false    |
| `CODER_INNER_HOSTNAME`     | The hostname to use for the inner container.                                                                                                                                                                                                                                                                                                                                                                                                                                    | false    |
| `CODER_IMAGE_PULL_SECRET`  | The docker credentials to use when pulling the inner container. The recommended way to do this is to create an [Image Pull Secret](https://kubernetes.io/docs/tasks/configure-pod-container/pull-image-private-registry/#registry-secret-existing-credentials) and then reference the secret using an [environment variable](https://kubernetes.io/docs/tasks/inject-data-application/distribute-credentials-secure/#define-container-environment-variables-using-secret-data). | false    |
| `CODER_DOCKER_BRIDGE_CIDR` | The bridge CIDR to start the Docker daemon with.                                                                                                                                                                                                                                                                                                                                                                                                                                | false    |
| `CODER_MOUNTS`             | A list of mounts to mount into the inner container. Mounts default to `rw`. Ex: `CODER_MOUNTS=/home/coder:/home/coder,/var/run/mysecret:/var/run/mysecret:ro`                                                                                                                                                                                                                                                                                                                   | false    |
| `CODER_USR_LIB_DIR`        | The mountpoint of the host `/usr/lib` directory. Only required when using GPUs.                                                                                                                                                                                                                                                                                                                                                                                                 | false    |
| `CODER_ADD_TUN`            | If `CODER_ADD_TUN=true` add a TUN device to the inner container.                                                                                                                                                                                                                                                                                                                                                                                                                | false    |
| `CODER_ADD_FUSE`           | If `CODER_ADD_FUSE=true` add a FUSE device to the inner container.                                                                                                                                                                                                                                                                                                                                                                                                              | false    |
| `CODER_ADD_GPU`            | If `CODER_ADD_GPU=true` add detected GPUs and related files to the inner container. Requires setting `CODER_USR_LIB_DIR` and mounting in the hosts `/usr/lib/` directory.                                                                                                                                                                                                                                                                                                       | false    |
| `CODER_CPUS`               | Dictates the number of CPUs to allocate the inner container. It is recommended to set this using the Kubernetes [Downward API](https://kubernetes.io/docs/tasks/inject-data-application/environment-variable-expose-pod-information/#use-container-fields-as-values-for-environment-variables).                                                                                                                                                                                 | false    |
| `CODER_MEMORY`             | Dictates the max memory (in bytes) to allocate the inner container. It is recommended to set this using the Kubernetes [Downward API](https://kubernetes.io/docs/tasks/inject-data-application/environment-variable-expose-pod-information/#use-container-fields-as-values-for-environment-variables).                                                                                                                                                                          | false    |

# Migrating Existing Envbox Templates

Due to the [deprecation and removal of legacy parameters](https://coder.com/docs/v2/latest/templates/parameters#legacy)
it may be necessary to migrate existing envbox templates on newer versions of
Coder. Consult the [migration](https://coder.com/docs/v2/latest/templates/parameters#migration)
documentation for details on how to do so.

To supply values to existing existing Terraform variables you can specify the
`-V` flag. For example

```bash
coder templates create envbox --var namespace="mynamespace" --var max_cpus=2 --var min_cpus=1 --var max_memory=4 --var min_memory=1
```

## Contributions

Contributions are welcome and can be made against the [envbox repo](https://github.com/coder/envbox).
