To use Coder you will need to install the Coder server on your infrastructure.
There are a number of different ways to install Coder, depending on your needs.

<children>
  This page is rendered on https://coder.com/docs/v2/latest/install. Refer to the other documents in the `install/` directory for per-platform instructions.
</children>

## Install Coder

<div class="tabs">

## Linux

<div class="tabs">

## Install Script

The easiest way to install Coder on Linux is to use our
[install script](https://github.com/coder/coder/blob/main/install.sh).

```shell
curl -fsSL https://coder.com/install.sh | sh
```

You can preview what occurs during the install process:

```shell
curl -fsSL https://coder.com/install.sh | sh -s -- --dry-run
```

You can modify the installation process by including flags. Run the help command
for reference:

```shell
curl -fsSL https://coder.com/install.sh | sh -s -- --help
```

## Homebrew

To install Coder on Linux, you can use the [Homebrew](https://brew.sh/) package
manager that uses our official
[Homebrew tap](https://github.com/coder/homebrew-coder).

```shell
brew install coder/coder/coder
```

## System Packages

Coder officially maintains packages for the following Linux distributions:

- .deb (Debian, Ubuntu)
- .rpm (Fedora, CentOS, RHEL, SUSE)
- .apk (Alpine)

<div class="tabs">

## Debian, Ubuntu

For Debian and Ubuntu, get the latest `.deb` package from our
[GitHub releases](https://github.com/coder/coder/releases/latest) and install it
manually or use the following commands to download and install the latest `.deb`
package.

```shell
# Install the package
sudo apt install ./coder.deb
```

## RPM Linux

For Fedora, CentOS, RHEL, SUSE, get the latest `.rpm` package from our
[GitHub releases](https://github.com/coder/coder/releases/latest) and install it
manually or use the following commands to download and install the latest `.rpm`
package.

```shell
# Install the package
sudo yum install ./coder.rpm
```

## Alpine

Get the latest `.apk` package from our
[GitHub releases](https://github.com/coder/coder/releases/latest) and install it
manually or use the following commands to download and install the latest `.apk`
package.

```shell
# Install the package
sudo apk add ./coder.apk
```

</div>

## Manual

Get the latest `.tar.gz` package from our
[GitHub releases](https://github.com/coder/coder/releases/latest) and install it
manually.

1. Download the
   [release archive](https://github.com/coder/coder/releases/latest) appropriate
   for your operating system

2. Unzip the folder you just downloaded, and move the `coder` executable to a
   location that's on your `PATH`

```shell
mv coder /usr/local/bin
```

</div>

## macOS

<div class="tabs">

## Homebrew

To install Coder on macOS, you can use the [Homebrew](https://brew.sh/) package
manager that uses our official
[Homebrew tap](https://github.com/coder/homebrew-coder).

```shell
brew install coder/coder/coder
```

## Install Script

The easiest way to install Coder on macOS is to use our
[install script](https://github.com/coder/coder/blob/main/install.sh).

```shell
curl -fsSL https://coder.com/install.sh | sh
```

You can preview what occurs during the install process:

```shell
curl -fsSL https://coder.com/install.sh | sh -s -- --dry-run
```

You can modify the installation process by including flags. Run the help command
for reference:

```shell
curl -fsSL https://coder.com/install.sh | sh -s -- --help
```

</div>

## Windows

<div class="tabs">

## Winget

To install Coder on Windows, you can use the
[`winget`](https://learn.microsoft.com/en-us/windows/package-manager/winget/#use-winget)
package manager.

```powershell
winget install Coder.Coder
```

## Installer

Download the Windows installer from our
[GitHub releases](https://github.com/coder/coder/releases/latest) and install
it.

## Manual

Get the latest `.zip` package from our GitHub releases page and extract it to a
location that's on your `PATH` or add the extracted binary to your `PATH`.

> Windows users: see
> [this guide](https://answers.microsoft.com/en-us/windows/forum/all/adding-path-variable/97300613-20cb-4d85-8d0e-cc9d3549ba23)
> for adding folders to `PATH`.

</div>

</div>

## Verify installation

Verify that the installation was successful by opening a new terminal and
running:

```console
coder --version
Coder v2.6.0+b3e3521 Thu Dec 21 22:33:13 UTC 2023
https://github.com/coder/coder/commit/b3e352127478bfd044a1efa77baace096096d1e6

Full build of Coder, supports the  server  subcommand.
...
```

## Start Coder

1. After installing, start the Coder server manually via `coder server` or as a
   system package.

    <div class="tabs">

   ## Terminal

   ```shell
   # Automatically sets up an external access URL on *.try.coder.app
   coder server

   # Requires a PostgreSQL instance (version 13 or higher) and external access URL
   coder server --postgres-url <url> --access-url <url>
   ```

   ## System Package (Linux)

   Run Coder as a system service.

   ```shell
   # (Optional) Set up an access URL
   sudo vim /etc/coder.d/coder.env

   # To systemd to start Coder now and on reboot
   sudo systemctl enable --now coder

   # View the logs to see Coder URL and ensure a successful start
   journalctl -u coder.service -b
   ```

    </div>

   > Set `CODER_ACCESS_URL` to the external URL that users and workspaces will
   > use to connect to Coder. This is not required if you are using the tunnel.
   > Learn more about Coder's [configuration options](../admin/configure.md).

   By default, the Coder server runs on `http://127.0.0.1:3000` and uses a
   [public tunnel](../admin/configure.md#tunnel) for workspace connections.

2. Visit the Coder URL in the logs to set up your first account, or use the CLI
   to create your first user.

   ```shell
   coder login <access url>
   ```

## Next steps

- [Configuring Coder](../admin/configure.md)
- [Templates](../templates/index.md)
