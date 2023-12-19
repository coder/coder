You can install and run Coder using the official Docker images published on
[GitHub Container Registry](https://github.com/coder/coder/pkgs/container/coder).

<blockquote class="warning">
**Before you install**
If you would like your workspaces to be able to run Docker, we recommend that you <a href="https://github.com/nestybox/sysbox#installation" target="_blank">install Sysbox</a> before proceeding.

As part of the Sysbox installation you will be required to remove all existing
Docker containers including containers used by Coder workspaces. Installing
Sysbox ahead of time will reduce disruption to your Coder instance.

</blockquote>

## Requirements

Docker is required. See the
[official installation documentation](https://docs.docker.com/install/).

> Note that the below steps are only supported on a Linux distribution. If on
> macOS, please [run Coder via the standalone binary](./binary.md).

## Run Coder with the built-in database (quick)

For proof-of-concept deployments, you can run a complete Coder instance with the
following command.

```console
export CODER_DATA=$HOME/.config/coderv2-docker
export DOCKER_GROUP=$(getent group docker | cut -d: -f3)
mkdir -p $CODER_DATA
docker run --rm -it \
  -v $CODER_DATA:/home/coder/.config \
  -v /var/run/docker.sock:/var/run/docker.sock \
  --group-add $DOCKER_GROUP \
  ghcr.io/coder/coder:latest
```

**<sup>Note:</sup>** <sup>Coder runs as a non-root user, we use `--group-add` to
ensure Coder has permissions to manage Docker via `docker.sock`. If the host
systems `/var/run/docker.sock` is not group writeable or does not belong to the
`docker` group, the above may not work as-is.</sup>

Coder configuration is defined via environment variables. Learn more about
Coder's [configuration options](../admin/configure.md).

## Run Coder with access URL and external PostgreSQL (recommended)

For production deployments, we recommend using an external PostgreSQL database
(version 13 or higher). Set `ACCESS_URL` to the external URL that users and
workspaces will use to connect to Coder.

```console
docker run --rm -it \
  -e CODER_ACCESS_URL="https://coder.example.com" \
  -e CODER_PG_CONNECTION_URL="postgresql://username:password@database/coder" \
  -v /var/run/docker.sock:/var/run/docker.sock \
  ghcr.io/coder/coder:latest
```

Coder configuration is defined via environment variables. Learn more about
Coder's [configuration options](../admin/configure.md).

## Run Coder with docker-compose

Coder's publishes a
[docker-compose example](https://github.com/coder/coder/blob/main/docker-compose.yaml)
which includes an PostgreSQL container and volume.

1. Install [Docker Compose](https://docs.docker.com/compose/install/)

2. Clone the `coder` repository:

   ```console
   git clone https://github.com/coder/coder.git
   ```

3. Start Coder with `docker-compose up`:

   In order to use cloud-based templates (e.g. Kubernetes, AWS), you must have
   an external URL that users and workspaces will use to connect to Coder.

   For proof-of-concept deployments, you can use
   [Coder's tunnel](../admin/configure.md#tunnel):

   ```console
   cd coder

   docker-compose up
   ```

   For production deployments, we recommend setting an
   [access URL](../admin/configure.md#access-url):

   ```console
   cd coder

   CODER_ACCESS_URL=https://coder.example.com docker-compose up
   ```

4. Visit the web ui via the configured url. You can add `/login` to the base url
   to create the first user via the ui.

5. Follow the on-screen instructions log in and create your first template and
   workspace

## Troubleshooting

### Docker-based workspace is stuck in "Connecting..."

Ensure you have an externally-reachable `CODER_ACCESS_URL` set. See
[troubleshooting templates](../templates/index.md#troubleshooting-templates) for
more steps.

### Permission denied while trying to connect to the Docker daemon socket

See Docker's official documentation to
[Manage Docker as a non-root user](https://docs.docker.com/engine/install/linux-postinstall/#manage-docker-as-a-non-root-user)

## Next steps

- [Configuring Coder](../admin/configure.md)
- [Templates](../templates/index.md)
