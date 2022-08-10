# Install

## install.sh

The easiest way to install Coder is to use our [install script](https://github.com/coder/coder/blob/main/install.sh) for Linux and macOS.

To install, run:

```bash
curl -fsSL https://coder.com/install.sh | sh
```

You can preview what occurs during the install process:

```bash
curl -fsSL https://coder.com/install.sh | sh -s -- --dry-run
```

You can modify the installation process by including flags. Run the help command for reference:

```bash
curl -fsSL https://coder.com/install.sh | sh -s -- --help
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

> See our [docker-compose](https://github.com/coder/coder/blob/main/docker-compose.yaml) file
> for additional information.

1. Clone the `coder` repository:

   ```console
   git clone https://github.com/coder/coder.git
   ```

2. Navigate into the `coder` folder and run `docker-compose up`:

   ```console
   cd coder
   # Coder will bind to localhost:7080.
   # You may use localhost:7080 as your access URL
   # when using Docker workspaces exclusively.
   #  CODER_ACCESS_URL=http://localhost:7080
   # Otherwise, an internet accessible access URL
   # is required.
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

If the user is not in the Docker group, you will see the following error:

```sh
Error: Error pinging Docker server: Got permission denied while trying to connect to the Docker daemon socket
```

The default docker socket only permits connections from `root` or members of the `docker`
group. Remedy like this:

```sh
# replace "coder" with user running coderd
sudo usermod -aG docker coder
grep /etc/group -e "docker"
sudo systemctl restart coder.service
```

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

## Up Next

Learn how to [configure](./install/configure.md) and [upgrade](./install/upgrade.md) Coder.
