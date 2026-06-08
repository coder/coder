# Upgrade

This article describes how to upgrade your Coder server.

> [!CAUTION]
> Prior to upgrading a production Coder deployment, take a database snapshot since
> Coder does not support rollbacks.

For upgrade recommendations and troubleshooting, see
[Upgrading Best Practices](./upgrade-best-practices.md).

## Configuration directory moved from `/etc/coder.d/` to `/etc/coder/`

Starting with this release, the deb and rpm packages install the server's
environment file at `/etc/coder/coder.env` instead of `/etc/coder.d/coder.env`.
The Unix `.d` suffix is reserved for drop-in directories whose contents are
merged at runtime, and Coder only reads a single env file, so the suffix was
misleading.

The package's postinstall script handles backward compatibility automatically:

- It creates `/etc/coder/` (owned by `coder:coder`) if it does not already
  exist.
- For every `*.env` file under `/etc/coder.d/`, it symlinks the file into
  `/etc/coder/` whenever the new location is empty or still holds the shipped
  default placeholder. Existing `/etc/coder/*.env` files with user content are
  never overwritten.
- It runs `systemctl daemon-reload` so the updated `EnvironmentFile=` path
  takes effect on the next start.

After the upgrade, restart the service to pick up the new unit:

```shell
sudo systemctl daemon-reload
sudo systemctl restart coder
```

You can continue editing `/etc/coder.d/coder.env` indefinitely; the symlink
makes the file visible at the new path. When you are ready to consolidate,
move the file's contents into `/etc/coder/coder.env` and delete the symlink
and the legacy directory:

```shell
sudo mv /etc/coder.d/coder.env /etc/coder/coder.env
sudo rmdir /etc/coder.d
```

> [!NOTE]
> On RPM-based systems with custom SELinux policy that explicitly labels
> `/etc/coder.d/`, copy the same file context onto `/etc/coder/` (for example
> with `semanage fcontext -a` and `restorecon`) before restarting the service.
> The default `etc_t` label is inherited automatically and needs no action.

## Reinstall Coder to upgrade

To upgrade your Coder server, reinstall Coder using your original method
of [install](../install/index.md).

### Coder install script

1. If you installed Coder using the `install.sh` script, re-run the below command
   on the host:

   ```shell
   curl -L https://coder.com/install.sh | sh
   ```

1. If you're running Coder as a system service, you can restart it with `systemctl`:

   ```shell
   systemctl daemon-reload
   systemctl restart coder
   ```

### Other upgrade methods

<div class="tabs">

### docker-compose

If you installed using `docker-compose`, run the below command to upgrade the
Coder container:

```shell
docker-compose pull coder && docker-compose up -d coder
```

### Kubernetes

See
[Upgrading Coder via Helm](../install/kubernetes.md#upgrading-coder-via-helm).

### Coder AMI on AWS

1. Run the Coder installation script on the host:

   ```shell
   curl -L https://coder.com/install.sh | sh
   ```

   The script will unpack the new `coder` binary version over the one currently
   installed.

1. Restart the Coder system process with `systemctl`:

   ```shell
   systemctl daemon-reload
   systemctl restart coder
   ```

### Windows

Download the latest Windows installer or binary from
[GitHub releases](https://github.com/coder/coder/releases/latest), or upgrade
from Winget.

```pwsh
winget install Coder.Coder
```

</div>
