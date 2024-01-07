1. Install Coder

   <div class="tabs">

   ## Install Script (Linux/macOS)

   The easiest way to install Coder is to use our
   [install script](https://github.com/coder/coder/blob/main/install.sh) for
   Linux and macOS.

   ```shell
   curl -fsSL https://coder.com/install.sh | sh
   ```

   You can preview what occurs during the install process:

   ```shell
   curl -fsSL https://coder.com/install.sh | sh -s -- --dry-run
   ```

   You can modify the installation process by including flags. Run the help
   command for reference:

   ```shell
   curl -fsSL https://coder.com/install.sh | sh -s -- --help
   ```

   ## Homebrew

   Install Coder from our official
   [Homebrew tap](https://github.com/coder/homebrew-coder)

   ```shell
   brew install coder/coder/coder
   ```

   ## Windows

   Install using
   [`winget`](https://learn.microsoft.com/en-us/windows/package-manager/winget/#use-winget)
   package manager

   ```powershell
   winget install Coder.Coder
   ```

   ## System Packages

   Download and install one of the following system packages from
   [GitHub releases](https://github.com/coder/coder/releases/latest) and install
   manullay

   - .deb (Debian, Ubuntu)
   - .rpm (Fedora, CentOS, RHEL, SUSE)
   - .apk (Alpine)
   - .exe (Windows)

   ## Binary

   1. Download the
      [release archive](https://github.com/coder/coder/releases/latest)
      appropriate for your operating system

   2. Unzip the folder you just downloaded, and move the `coder` executable to a
      location that's on your `PATH`

   ```shell
   # ex. macOS and Linux
   mv coder /usr/local/bin
   ```

   > Windows users: see
   > [this guide](https://answers.microsoft.com/en-us/windows/forum/all/adding-path-variable/97300613-20cb-4d85-8d0e-cc9d3549ba23)
   > for adding folders to `PATH`.

   </div>

2. After installing, start the Coder server manually via `coder server` or as a
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

3. Visit the Coder URL in the logs to set up your first account, or use the CLI
   to create your first user.

   ```shell
   coder login <access url>
   ```

There are a number of other different methods to install and run Coder:

<children>
  This page is rendered on https://coder.com/docs/v2/latest/install. Refer to the other documents in the `install/` directory for per-platform instructions.
</children>

## Next steps

- [Configuring Coder](../admin/configure.md)
- [Templates](../templates/index.md)
