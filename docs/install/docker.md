Coder publishes containerized images

## Standalone container

Requirements:

- Docker
-

   sudo rm -r ~/coder-stuff2/
   mkdir ~/coder-stuff2/
   docker run --rm -it \
    -e CODER_TUNNEL=true \
    -e CODER_CONFIG_DIR=/opt/coder-data \
   -v /var/run/docker.sock:/var/run/docker.sock \
    -v $HOME/coder-stuff2/:/opt/coder-data/ \
   ghcr.io/coder/coder:latest

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

   docker run --rm -it \
    -e CODER_TUNNEL=true \
    -v /var/run/docker.sock:/var/run/docker.sock \
    -v ~/idk:/home/coder \
   ghcr.io/coder/coder:latest

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
