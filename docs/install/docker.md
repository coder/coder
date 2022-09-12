You can install and run Coder using the official Docker images published on [GitHub Container Registry](https://github.com/coder/coder/pkgs/container/coder).

## Requirements

Docker is required. See the [official installation documentation](https://docs.docker.com/install/).

## Run Coder with built-in database and tunnel (quick)

For proof-of-concept deployments, you can run a complete Coder instance with
with the following command:

```sh
export CODER_DATA=$HOME/.config/coderv2-docker
mkdir -p $CODER_DATA
docker run --rm -it \
  -e CODER_TUNNEL=true \
  -v $CODER_DATA:/home/coder/.config \
  -v /var/run/docker.sock:/var/run/docker.sock \
  ghcr.io/coder/coder:latest
```

Coder configuration is defined via environment variables.
Learn more about Coder's [configuration options](../admin/configure.md).

## Run Coder with access URL and external PostgreSQL (recommended)

For production deployments, we recommend using an external PostgreSQL database.
Set `ACCESS_URL` to the external URL that users and workspaces will use to
connect to Coder.

```sh
docker run --rm -it \
  -e CODER_ACCESS_URL="https://coder.example.com" \
  -e CODER_PG_CONNECTION_URL="postgresql://username:password@database/coder" \
  -v /var/run/docker.sock:/var/run/docker.sock \
  ghcr.io/coder/coder:latest
```

Coder configuration is defined via environment variables.
Learn more about Coder's [configuration options](../admin/configure.md).

## Run Coder with docker-compose

Coder's publishes a [docker-compose example](https://github.com/coder/coder/blob/main/docker-compose.yaml) which includes
an PostgreSQL container and volume.

1. Install [Docker Compose](https://docs.docker.com/compose/install/)

2. Clone the `coder` repository:

   ```console
   git clone https://github.com/coder/coder.git
   ```

3. Start Coder with `docker-compose up`:

   In order to use cloud-based templates (e.g. Kubernetes, AWS), you must set `CODER_ACCESS_URL` to the external URL that users and workspaces will use to connect to Coder.

   ```console
   cd coder

   CODER_ACCESS_URL=https://coder.example.com
   docker-compose up
   ```

   > Without `CODER_ACCESS_URL` set, Coder will bind to `localhost:7080`. This will only work for Docker-based templates.

4. Follow the on-screen instructions log in and create your first template and workspace

## Troubleshooting

### Docker-based workspace is stuck in "Connecting..."

Ensure you have an externally-reachable `CODER_ACCESS_URL` set. See [troubleshooting templates](../templates.md#creating-and-troubleshooting-templates) for more steps.

### Permission denied while trying to connect to the Docker daemon socket

See Docker's official documentation to [Manage Docker as a non-root user](https://docs.docker.com/engine/install/linux-postinstall/#manage-docker-as-a-non-root-user)

## Next steps

- [Quickstart](../quickstart.md)
- [Configuring Coder](../admin/configure.md)
- [Templates](../templates.md)
