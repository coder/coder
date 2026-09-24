---
title: Install Coder on RHEL-family Linux
---

This guide is for a Coder deployment administrator who runs the Coder control plane on a single Red Hat Enterprise Linux host or another RHEL-family distribution.
Coder publishes an RPM with every release, and `systemd` manages the server like any other service.
For a multi-replica deployment, refer to [Kubernetes](./kubernetes/index.md) or [OpenShift](./openshift.md) instead.
To install the CLI on a workstation that connects to an existing deployment, refer to [Install the Coder CLI](../cli.md).

## Supported distributions

Coder builds one RPM per release and architecture.
The install script reads `/etc/os-release` and selects the RPM when `ID` or `ID_LIKE` names a Fedora-family or openSUSE-family distribution.

| Distribution             | Versions         | `ID`                                   |
|--------------------------|------------------|----------------------------------------|
| Red Hat Enterprise Linux | 8, 9, 10         | `rhel`                                 |
| Rocky Linux              | 8, 9, 10         | `rocky`                                |
| AlmaLinux                | 8, 9, 10         | `almalinux`                            |
| CentOS Stream            | 9, 10            | `centos`                               |
| Amazon Linux             | 2, 2023          | `amzn`                                 |
| Fedora                   | Current releases | `fedora`                               |
| openSUSE                 | Leap, Tumbleweed | `opensuse-leap`, `opensuse-tumbleweed` |

Every RHEL-family distribution in the table lists `fedora` in `ID_LIKE`, and both openSUSE variants list `opensuse`.
A derivative that isn't listed here takes the same path when its `ID_LIKE` includes `fedora`.
Each release publishes RPMs for `amd64`, `arm64`, and `armv7`.

## Requirements

- A host running one of the distributions above.
- 2 CPU cores and 4&nbsp;GB of memory to evaluate Coder.
  For production sizing, refer to the [Coder Validated Architecture](../plan/sizing/index.md).
- `root` or `sudo` access.
- An external PostgreSQL database for anything beyond a proof of concept.
  Refer to [Using an external database](../../tutorials/external-database.md).

## Install the package

The install script detects the distribution and installs the matching RPM.
Install the package directly with `dnf` or `rpm` when the host can't run a piped shell script, or when you stage packages yourself.

<div class="tabs">

### Install script

```sh
curl -fsSL https://coder.com/install.sh | sh
```

The script installs the latest mainline release.
Pass `--stable` for the latest stable release, or `--version X.Y.Z` for a specific version.
Pass `--dry-run` to print the commands without running them.

### dnf

1. Download the RPM for your version and architecture from [GitHub releases](https://github.com/coder/coder/releases).
2. Install the package.

   ```sh
   sudo dnf install ./coder_<version>_linux_amd64.rpm
   ```

On hosts that use `yum`, run `sudo yum install` instead.

### rpm

Install or upgrade the package in place.
This is the command the install script runs.

```sh
sudo rpm -U coder_<version>_linux_amd64.rpm
```

</div>

The package installs the `coder` binary to `/usr/bin/coder`, the configuration file to `/etc/coder.d/coder.env`, and two `systemd` units: `coder.service` for the control plane and `coder-workspace-proxy.service` for an optional [workspace proxy](../../admin/networking/workspace-proxies.md).
It also creates the unprivileged `coder` user that the service runs as.

To stage the RPM on a host with no internet access, refer to [Air-gapped deployments](../prepare/airgap.md).

## Configure the server

`coder.service` doesn't start while `/etc/coder.d/coder.env` is empty.
Set your configuration before you start the service.

1. Open the configuration file.

   ```sh
   sudo vi /etc/coder.d/coder.env
   ```

2. Set `CODER_ACCESS_URL` to the external URL that users and workspaces connect to.
3. Set `CODER_PG_CONNECTION_URL` to your PostgreSQL connection string.
4. Save the file.

The file ships with the TLS and HTTP address variables commented in place.
For every server option, refer to the [configuration reference](../../admin/setup/configuration-reference.md).

## Start the service

1. Start Coder now and on every boot.

   ```sh
   sudo systemctl enable --now coder
   ```

2. Confirm that the service is running.

   ```console
   $ systemctl is-active coder
   active
   ```

3. Read the startup logs.

   ```sh
   journalctl -u coder.service -b
   ```

To test a configuration change without the service manager, run `coder server` in the foreground.

## SELinux and container runtimes in workspaces

SELinux affects workspaces that run their own container runtime.
If your templates run Docker or Podman inside a workspace on a RHEL-family node, refer to [Docker in workspaces](../../admin/templates/extending-templates/docker-in-workspaces.md) for the runtime requirements.

## Hardened base images

Coder publishes a container image built on Red Hat's UBI9-minimal base through [Iron Bank](https://ironbank.dso.mil/), the Department of Defense hardened container registry.
Choose that image when your compliance program requires an accredited base image.
The Iron Bank image is a container image, so deploy it with [Kubernetes](./kubernetes/index.md) or [OpenShift](./openshift.md) rather than through the RPM.

## Learn more

- [Validate your deployment](../validate/index.md)
- [Set up your control plane](../../admin/setup/index.md)
- [Create your first template](../../tutorials/template-from-scratch.md)
- [Upgrade Coder](../operate/upgrade.md)
- [Uninstall Coder](../operate/uninstall.md)
