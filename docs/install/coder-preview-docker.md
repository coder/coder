# Install and test coder-preview

Use Docker to install and test a
[preview release of Coder](https://github.com/coder/coder/pkgs/container/coder-preview) using Docker on Linux or Mac.

These steps are not intended for use in production deployments.
If you want to install the latest version of Coder, use the [quickstart guide](../tutorials/quickstart.md).

1. Install Docker:

    ```bash
    curl -sSL https://get.docker.com | sh
    ```

    For more details, visit:

    - [Linux instructions](https://docs.docker.com/desktop/install/linux-install/)
    - [Mac instructions](https://docs.docker.com/desktop/install/mac-install/)

1. Assign your user to the Docker group:

    ```shell
    sudo usermod -aG docker $USER
    ```

1. Run `newgrp` to activate the groups changes:

    ```shell
    newgrp docker
    ```

    You might need to log out and back in or restart the machine for changes to take effect.

1. Install Coder via `docker run` with latest preview from [coder-preview](https://github.com/coder/coder/pkgs/container/coder-preview):

   1. ```shell
      export CODER_DATA=$HOME/.config/coderv2-docker
      ```

   1. ```shell
      export DOCKER_GROUP=$(getent group docker | cut -d: -f3)
      ```

   1. ```shell
      mkdir -p $CODER_DATA
      ```

   1. ```shell
      docker run --rm -it \
        -v $CODER_DATA:/home/coder/.config \
        -v /var/run/docker.sock:/var/run/docker.sock \
        --group-add $DOCKER_GROUP \
        ghcr.io/coder/coder-preview:pr16309
      ```
