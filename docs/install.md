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

## Kubernetes via Helm

Before proceeding, please ensure that you have both Helm 3.5+ and the
[latest version of Coder](https://github.com/coder/coder/releases) installed.
You will also need to have a Kubernetes cluster running K8s 1.19+.

> See our [Helm README](https://github.com/coder/coder/blob/main/helm#readme)
> file for additional information. Check the
> [values.yaml](https://github.com/coder/coder/blob/main/helm/values.yaml) file
> for a list of supported Helm values and their defaults.

> ⚠️ **Warning**: Helm support is new and not yet complete. There may be changes
> to the Helm chart between releases which require manual values updates. Please
> file an issue if you run into any issues.
>
> Additionally, the Helm chart does not currently automatically configure a
> Service Account and workspace template for use in Coder. See
> [#3265](https://github.com/coder/coder/issues/3265).

1. Create a namespace for Coder, such as `coder`:

    ```console
    $ kubectl create namespace coder
    ```

1. Create a PostgreSQL deployment. Coder does not manage a database server for
   you.

    - If you're in a public cloud such as
      [Google Cloud](https://cloud.google.com/sql/docs/postgres/),
      [AWS](https://aws.amazon.com/rds/postgresql/),
      [Azure](https://docs.microsoft.com/en-us/azure/postgresql/), or
      [DigitalOcean](https://www.digitalocean.com/products/managed-databases-postgresql),
      you can use the managed PostgreSQL offerings they provide. Make sure that
      the PostgreSQL service is running and accessible from your cluster. It
      should be in the same network, same project, etc.

    - You can install Postgres manually on your cluster using the
      [Bitnami PostgreSQL Helm chart](https://github.com/bitnami/charts/tree/master/bitnami/postgresql#readme). There are some
      [helpful guides](https://phoenixnap.com/kb/postgresql-kubernetes) on the
      internet that explain sensible configurations for this chart. Example:

      ```console
      $ helm repo add bitnami https://charts.bitnami.com/bitnami
      $ helm install postgres bitnami/postgresql \
          --namespace coder \
          --set auth.username=coder \
          --set auth.password=coder \
          --set auth.database=coder \
          --set persistence.size=10Gi
      ```

      The cluster-internal DB URL for the above database is:
      ```
      postgres://coder:coder@postgres-postgresql.coder.svc.cluster.local:5432/coder?sslmode=disable
      ```

      > Ensure you set up periodic backups so you don't lose data.

    - You can use
      [Postgres operator](https://github.com/zalando/postgres-operator) to
      manage PostgreSQL deployments on your Kubernetes cluster.

1. Download the latest `coder_helm` package from
   [GitHub releases](https://github.com/coder/coder/releases).

1. Create a `values.yaml` with the configuration settings you'd like for your
   deployment. For example:

    ```yaml
    coder:
      # You can specify any environment variables you'd like to pass to Coder
      # here. Coder consumes environment variables listed in
      # `coder server --help`, and these environment variables are also passed
      # to the workspace provisioner (so you can consume them in your Terraform
      # templates for auth keys etc.).
      #
      # Please keep in mind that you should not set `CODER_ADDRESS`,
      # `CODER_TLS_ENABLE`, `CODER_TLS_CERT_FILE` or `CODER_TLS_KEY_FILE` as
      # they are already set by the Helm chart and will cause conflicts.
      env:
        - name: CODER_ACCESS_URL
          value: "https://coder.example.com"
        - name: CODER_PG_CONNECTION_URL
          valueFrom:
            secretKeyRef:
              # You'll need to create a secret called coder-db-url with your
              # Postgres connection URL like:
              # postgres://coder:password@postgres:5432/coder?sslmode=disable
              name: coder-db-url
              key: url

        # This env variable controls whether or not to auto-import the
        # "kubernetes" template on first startup. This will not work unless
        # coder.serviceAccount.workspacePerms is true.
        - name: CODER_TEMPLATE_AUTOIMPORT
          value: "kubernetes"

      tls:
        secretName: my-tls-secret-name
    ```

    > You can view our
    > [Helm README](https://github.com/coder/coder/blob/main/helm#readme) for
    > details on the values that are available, or you can view the
    > [values.yaml](https://github.com/coder/coder/blob/main/helm/values.yaml)
    > file directly.

1. Run the following commands to install the chart in your cluster.

    ```console
    $ helm install coder ./coder_helm_x.y.z.tgz \
        --namespace coder \
        --values values.yaml
    ```

You can watch Coder start up by running `kubectl get pods`. Once Coder has
started, the `coder-*` pods should enter the `Running` state.

You can view Coder's logs by getting the pod name from `kubectl get pods` and
then running `kubectl logs <pod name>`. You can also view these logs in your
Cloud's log management system if you are using managed Kubernetes.

To upgrade Coder in the future, you can run the following command with a new `coder_helm_x.y.z.tgz` file from GitHub releases:

```console
$ helm upgrade coder ./coder_helm_x.y.z.tgz \
    --namespace coder \
    -f values.yaml
```

## Manual

We publish self-contained .zip and .tar.gz archives in [GitHub releases](https://github.com/coder/coder/releases). The archives bundle `coder` binary.

1. Download the [release archive](https://github.com/coder/coder/releases) appropriate for your operating system

1. Unzip the folder you just downloaded, and move the `coder` executable to a location that's on your `PATH`

   ```sh
   # ex. macOS and Linux
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

- Learn how to [configure](./install/configure.md) Coder.
- Learn about [upgrading](./install/upgrade.md) Coder.
