---
title: Install Coder on RHEL-family Linux
---

Coder publishes an RPM package with every release, so you can run the Coder
control plane directly on a Red Hat Enterprise Linux host or any RHEL-family
distribution, managed by `systemd` like any other service.

Use this guide when you want a single Linux host running the control plane. For
multi-replica or high-availability deployments, install Coder on
[Kubernetes](./kubernetes.md) or [OpenShift](./openshift.md) instead.

> [!NOTE]
> This page covers the Coder server. To install the CLI on a workstation that
> connects to an existing deployment, see [Install the Coder CLI](./cli.md).

## Supported distributions

Coder builds one RPM per release and architecture. The install script selects it
based on the `ID` and `ID_LIKE` fields in `/etc/os-release`, so RHEL derivatives
are detected automatically.

| Distribution              | Versions         | `ID` in `/etc/os-release`              |
|---------------------------|------------------|----------------------------------------|
| Red Hat Enterprise Linux  | 8, 9, 10         | `rhel`                                 |
| Rocky Linux               | 8, 9, 10         | `rocky`                                |
| AlmaLinux                 | 8, 9, 10         | `almalinux`                            |
| CentOS Stream             | 9, 10            | `centos`                               |
| Amazon Linux              | 2, 2023          | `amzn`                                 |
| Fedora                    | Current releases | `fedora`                               |
| openSUSE Leap, Tumbleweed | Current releases | `opensuse-leap`, `opensuse-tumbleweed` |

Every RHEL-family entry above declares `fedora` in its `ID_LIKE` field, which is
what the install script matches on, so derivatives not listed here generally
work the same way.

RPMs are published for `amd64`, `arm64`, and `armv7`.

## Requirements

- A host running one of the distributions above.
- 2 CPU cores and 4 GB of memory for an evaluation deployment. For production
  sizing, see the
  [validated architectures](../admin/infrastructure/validated-architectures/index.md).
- `root` or `sudo` access to install the package and manage the service.
- An external PostgreSQL database for anything beyond a proof of concept. See
  [Using an external database](../tutorials/external-database.md).

## Install the package

<div class="tabs">

## Install script

The install script detects your distribution, downloads the matching RPM from
the GitHub release, and installs it:

```shell
curl -L https://coder.com/install.sh | sh
```

To install a specific release channel or version, pass `--stable` or
`--version X.Y.Z`. Run the script with `--dry-run` to print the commands it would
run without running them.

## dnf

Download the RPM for your version and architecture from
[GitHub releases](https://github.com/coder/coder/releases), then install it with
your package manager so dependencies resolve normally:

```shell
sudo dnf install ./coder_<version>_linux_amd64.rpm
```

On hosts that still use `yum`, substitute `sudo yum install`.

## rpm

If you only need the package installed or upgraded in place, use `rpm` directly.
This is what the install script runs:

```shell
sudo rpm -U coder_<version>_linux_amd64.rpm
```

</div>

Both package managers work in air-gapped environments if you stage the RPM on
the host first. See [Air-gapped deployments](./airgap.md).

## Configure and start the service

The package installs a `systemd` unit and reads its configuration from
`/etc/coder.d/coder.env`. Edit that file to set your access URL, database
connection string, and any other
[server options](../admin/setup/index.md):

```shell
sudo vi /etc/coder.d/coder.env
```

Start Coder now and on every boot:

```shell
sudo systemctl enable --now coder
```

Check that the service came up:

```shell
journalctl -u coder.service -b
```

You can also run the server in the foreground with `coder server`, which is
useful when you're testing configuration changes.

## Hardened and regulated environments

For environments that require an accredited base image, Coder publishes a
container image built on Red Hat's UBI9-minimal base through
[Iron Bank](https://ironbank.dso.mil/), the DoD hardened container registry. Use
that image when your compliance program requires a hardened, scanned base rather
than a package installed on a host you manage.

The Iron Bank image is a container image: run it on
[Kubernetes](./kubernetes.md) or [OpenShift](./openshift.md), not through the
RPM path described on this page.

## SELinux and workspaces on RHEL nodes

Installing and running the control plane works with SELinux in enforcing mode
and needs no policy changes.

SELinux does affect workspaces that run a container runtime inside themselves.
If your templates use Docker or Podman inside a workspace on a RHEL-family node,
you might need to set SELinux to permissive mode or add a policy for the
runtime. See
[Docker in workspaces](../admin/templates/extending-templates/docker-in-workspaces.md)
for the runtime-specific requirements.

## Next steps

- [Set up your control plane](../admin/setup/index.md)
- [Create your first template](../tutorials/template-from-scratch.md)
- [Upgrade Coder](./upgrade.md)
- [Uninstall Coder](./uninstall.md)
