# Install

## install.sh

The easiest way to install Coder is to use our [install script](https://github.com/coder/coder/blob/main/install.sh) for Linux and macOS.

To install, run:

```bash
curl -L https://coder.com/install.sh | sh
```

You can preview what occurs during the install process:

```bash
curl -L https://coder.com/install.sh | sh -s -- --dry-run
```

You can modify the installation process by including flags. Run the help command for reference:

```bash
curl -L https://coder.com/install.sh | sh -s -- --help
```

## System packages

Coder publishes the following system packages [in GitHub releases](https://github.com/coder/coder/releases):

- .deb (Debian, Ubuntu)
- .rpm (Fedora, CentOS, RHEL, SUSE)
- .apk (Alpine)

Once installed, you can run Coder as a system service:

```sh
 # Set up an external access URL or enable CODER_TUNNEL
sudo vim /etc/coder.d/coder.env
# Use systemd to start Coder now and on reboot
sudo systemctl enable --now coder
# View the logs to ensure a successful start
journalctl -u coder.service -b
```

## docker-compose

Before proceeding, please ensure that you have both Docker and the [latest version of
Coder](https://github.com/coder/coder/releases) installed.

> See our [docker-compose](https://github.com/coder/coder/blob/93b78755a6d48191cc53c82654e249f25fc00ce9/docker-compose.yaml) file
> for additional information.

1. Clone the `coder` repository:

   ```console
   git clone https://github.com/coder/coder.git
   ```

2. Navigate into the `coder` folder. Coder requires a non-`localhost` access URL
   for non-Docker-based examples; if you have a public IP or a domain/reverse
   proxy, you can provide this value before running `docker-compose up` to
   start the service:

   ```console
   cd coder
   CODER_ACCESS_URL=https://coder.mydomain.com
   docker-compose up
   ```

   Otherwise, you can start the service:

   ```console
   cd coder
   docker-compose up
   ```

   Alternatively, if you would like to start a **temporary deployment**:

   ```console
   docker run --rm -it \
   -e CODER_DEV_MODE=true \
   -v /var/run/docker.sock:/var/run/docker.sock \
   ghcr.io/coder/coder:v0.5.10
   ```

3. Follow the on-screen instructions to create your first template and workspace

## Manual

We publish self-contained .zip and .tar.gz archives in [GitHub releases](https://github.com/coder/coder/releases). The archives bundle `coder` binary.

1. Download the [release archive](https://github.com/coder/coder/releases) appropriate for your operating system

1. Unzip the folder you just downloaded, and move the `coder` executable to a location that's on your `PATH`

   ```sh
   # ex. MacOS and Linux
   mv coder /usr/local/bin
   ```

   > Windows users: see [this guide](https://answers.microsoft.com/en-us/windows/forum/all/adding-path-variable/97300613-20cb-4d85-8d0e-cc9d3549ba23) for adding folders to `PATH`.

1. Start a Coder server

    ```sh
    # Automatically sets up an external access URL on *.try.coder.app
    coder server --tunnel

    # Requires a PostgreSQL instance and external access URL
    coder server --postgres-url <url> --access-url <url>
    ```

## Next steps

Once you've installed and started Coder, see the [quickstart](./quickstart.md)
for instructions on creating your first template and workspace.
