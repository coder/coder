# Install

This article walks you through the various ways of installing and deploying Coder.

## install.sh

The easiest way to install Coder is to use our [install script](https://github.com/coder/coder/main/install.sh) for Linux and macOS. The install script
attempts to use the system package manager detection-reference if possible.

You can preview what occurs during the install process:

```bash
curl -fsSL https://coder.com/install.sh | sh -s -- --dry-run
```

To install, run:

```bash
curl -fsSL https://coder.com/install.sh | sh
```

> If you're concerned about the install script's use of `curl | sh` and the
> security implications, please see [this blog
> post](https://sandstorm.io/news/2015-09-24-is-curl-bash-insecure-pgp-verified-install)
> by [sandstorm.io](https://sandstorm.io).

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
# Specify a PostgreSQL database
# in the configuration first:
sudo vim /etc/coder.d/coder.env
sudo service coder restart
```

Or run a **temporary deployment** with dev mode (all data is in-memory and destroyed on exit):


```sh
coder server --dev
```

## docker-compose

Before proceeding, please ensure that you have both Docker and the [latest version of
Coder](https://github.com/coder/coder/releases) installed.

1. Clone the `coder` repository:

    ```console
    git clone git@github.com:coder/coder.git
    ```

1. Navigate into the `coder` folder. Coder requires a non-`localhost` access URL
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

1. Open a new terminal window, and run `coder login <yourAccessURL>` to create
   your first user (once you've done so, you can navigate to `yourAccessURL` and
   log in with these credentials).

1. Next, copy a sample template into a new directory so that you can create a custom template in a
   subsequent step (be sure that you're working in the directory where you want
   your templates stored):

   ```console
   coder templates init
   ```

   Choose the "Develop in Docker" example to generate a sample template in the
   `docker` subdirectory.

1. Navigate into the new directory and create a new template:

    ```console
    cd docker
    coder templates create
    ```

    Follow the prompts displayed to proceed. When done, you'll see the following
    message:

    ```console
    The docker template has been created! Developers can
    provision a workspace with this template using:

    coder create --template="docker" [workspace name]
    ```

1. At this point, you're ready to provision your first workspace:

    ```console
    coder create --template="docker" [workspace name]
    ```

    Follow the on-screen prompts to set the parameters for your workspace. If
    the process is successful, you'll get information regarding your workspace:

    ```console
    ┌─────────────────────────────────────────────────────────────────┐
    │ RESOURCE                    STATUS             ACCESS           │
    ├─────────────────────────────────────────────────────────────────┤
    │ docker_container.workspace  ephemeral                           │
    │ └─ dev (linux, amd64)       ⦾ connecting [0s]   coder ssh main  │
    ├─────────────────────────────────────────────────────────────────┤
    │ docker_volume.coder_volume  ephemeral                           │
    └─────────────────────────────────────────────────────────────────┘
    The main workspace has been created!
    ```

You can now access your workspace via your web browser by navigating to your
access URL, or you can connect to it via SSH by running:

```console
coder ssh [workspace name]
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

    To run a **temporary deployment**, start with dev mode (all data is in-memory and destroyed on exit):

    ```bash
    coder server --dev
    ```

    To run a **production deployment** with PostgreSQL:

    ```bash
    CODER_PG_CONNECTION_URL="postgres://<username>@<host>/<database>?password=<password>" \
      coder server
    ```
