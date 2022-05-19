# Install

This article walks you through the various ways of installing and deploying Coder.

## Docker Compose

Before proceeding, please ensure that you have Docker installed.

1. Clone the `coder` repository:

    ```console
    git clone git@github.com:coder/coder.git
    ```

1. Navigate into the `coder` folder. Coder requires a non-`localhost` access URL
    for non-Docker-based examples; if you have a public IP or a domain/reverse
    proxy, you can provide this value prior to running `docker-compose up` to
    start the service:

    ```console
    cd coder
    CODER_ACCESS_URL=https://coder.mydomain.com
    docker-compose up
    ```

    Otherwise, you can simply start the service:

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

1. Follow the on-screen prompts to create your first user and workspace.
