# Installing Coder

A single CLI (`coder`) is used for both the Coder server and the client.

We support two release channels: mainline and stable - read the
[Releases](./releases/index.md) page to learn more about which best suits your team.

There are several ways to install Coder. Follow the steps on this page for a
minimal installation of Coder, or for a step-by-step guide on how to install and
configure your first Coder deployment, follow the
[quickstart guide](../tutorials/quickstart.md).

## Local/Individual Installs

This install guide is meant for **individual developers, small teams, and/or open source community members** setting up Coder locally or on a single server. It covers the light weight install for Linux, macOS, and Windows.

<div class="tabs">

## Linux/macOS

Our install script is the fastest way to install Coder on Linux/macOS:

```sh
curl -L https://coder.com/install.sh | sh
```

Refer to [GitHub releases](https://github.com/coder/coder/releases) for
alternate installation methods (e.g. standalone binaries, system packages).

If you're using an Apple Silicon Mac with ARM64 architecture, so M1/M2/M3/M4, you'll need to use an external PostgreSQL Database using the following commands:

``` bash
# Install PostgreSQL
brew install postgresql@16

# Start PostgreSQL
brew services start postgresql@16

# Create database
createdb coder

# Run Coder with external database
coder server --postgres-url="postgres://$(whoami)@localhost/coder?sslmode=disable"
```

## Windows

If you plan to use the built-in PostgreSQL database, ensure that the
[Visual C++ Runtime](https://learn.microsoft.com/en-US/cpp/windows/latest-supported-vc-redist#latest-microsoft-visual-c-redistributable-version)
is installed.

Use [GitHub releases](https://github.com/coder/coder/releases) to download the
Windows installer (`.msi`) or standalone binary (`.exe`).

![Windows setup wizard](../images/install/windows-installer.png)

Alternatively, you can use the
[`winget`](https://learn.microsoft.com/en-us/windows/package-manager/winget/#use-winget)
package manager to install Coder:

```powershell
winget install Coder.Coder
```

</div>

## Hosted/Enterprise Installs

This install guide is meant for **IT Administrators, DevOps, and Platform Teams** deploying Coder for an organization. It covers production-grade, multi-user installs on Kubernetes and other hosted platforms.

<div>

<children></children>

</div>

## Starting the Coder Server

To start the Coder server:

```sh
coder server
```

![Coder install](../images/screenshots/welcome-create-admin-user.png)

To log in to an existing Coder deployment:

```sh
coder login https://coder.example.com
```
