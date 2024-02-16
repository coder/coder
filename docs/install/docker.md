You can install and run Coder using the official Docker images published on
[GitHub Container Registry](https://github.com/coder/coder/pkgs/container/coder).

## Requirements

Docker is required. See the
[official installation documentation](https://docs.docker.com/install/).

> Note that the below steps are only supported on a Linux distribution. If on
> macOS, please [run Coder via the standalone binary](./index.md#manual).

<div class="tabs">

## docker run

**Built-in database (quick)**

For proof-of-concept deployments, you can run a complete Coder instance with the
following command.

```shell
export CODER_DATA=$HOME/.config/coderv2-docker
export DOCKER_GROUP=$(getent group docker | cut -d: -f3)
mkdir -p $CODER_DATA
docker run --rm -it \
  -v $CODER_DATA:/home/coder/.config \
  -v /var/run/docker.sock:/var/run/docker.sock \
  --group-add $DOCKER_GROUP \
  ghcr.io/coder/coder:latest
```

**External database**

For production deployments, we recommend using an external PostgreSQL database
(version 13 or higher). Set `CODER_ACCESS_URL` to the external URL that users
and workspaces will use to connect to Coder.

```shell
export DOCKER_GROUP=$(getent group docker | cut -d: -f3)
docker run --rm -it \
  -e CODER_ACCESS_URL="https://coder.example.com" \
  -e CODER_PG_CONNECTION_URL="postgresql://username:password@database/coder" \
  -v /var/run/docker.sock:/var/run/docker.sock \
  --group-add $DOCKER_GROUP \
  ghcr.io/coder/coder:latest
```

## docker compose

Coder's publishes a
[docker-compose example](https://github.com/coder/coder/blob/main/docker-compose.yaml)
which includes an PostgreSQL container and volume.

1. Make sure you have [Docker Compose](https://docs.docker.com/compose/install/)
   installed.

2. Download the
   [`docker-compose.yaml`](https://github.com/coder/coder/blob/main/docker-compose.yaml)
   file.

3. Update `group_add:` in `docker-compose.yaml` with the `gid` of `docker`
   group. You can get the `docker` group `gid` by running the below command:

   ```shell
   getent group docker | cut -d: -f3
   ```

4. Start Coder with `docker compose up`

5. Visit the web ui via the configured url.

6. Follow the on-screen instructions log in and create your first template and
   workspace

</div>

Coder configuration is defined via environment variables. Learn more about
Coder's [configuration options](../admin/configure.md).

> **Note:** In order to use cloud-based templates (e.g. Kubernetes, AWS), you
> must have an external URL that users and workspaces will use to connect to
> Coder.
>
> > For proof-of-concept deployments, you can use
> > [Coder's tunnel](../admin/configure.md#tunnel).
> >
> > For production deployments, we recommend setting an
> > [access URL](../admin/configure.md#access-url)

> **Note:** Coder runs as a non-root user, we use `--group-add` to ensure Coder
> has permissions to manage Docker via `docker.sock`. If the host systems
> `/var/run/docker.sock` is not group writeable or does not belong to the
> `docker` group, the above may not work as-is.

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
