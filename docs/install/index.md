# Installing Coder

A single CLI (`coder`) is used for both the Coder server and the client.

We support two release channels: mainline and stable - read the
[Releases](./releases.md) page to learn more about which best suits your team.

There are several ways to install Coder. For production deployments with 50+
users, we recommend [installing on Kubernetes](./kubernetes.md). Otherwise, you
can install Coder on your local machine or on a VM:

<div class="tabs">

## Linux/macOS

Our install script is the fastest way to install Coder on Linux/macOS:

```sh
curl -L https://coder.com/install.sh | sh
```

Refer to [GitHub releases](https://github.com/coder/coder/releases) for
alternate installation methods (e.g. standalone binaries, system packages).

## Windows

Use [GitHub releases](https://github.com/coder/coder/releases) to download the
Windows installer (`.msi`) or standalone binary (`.exe`).

![Windows setup wizard](../images/install/windows-installer.png)

Alternatively, you can use the
[`winget`](https://learn.microsoft.com/en-us/windows/package-manager/winget/#use-winget)
package manager to install Coder:

```powershell
winget install Coder.Coder
```

## Other

<children></children>

</div>

To start the Coder server:

```sh
coder server
```

![Coder install](../images/install/coder-setup.png)

To log in to an existing Coder deployment:

```sh
coder login https://coder.example.com
```

## Next up

- [Create your first template](../templates/tutorial.md)
