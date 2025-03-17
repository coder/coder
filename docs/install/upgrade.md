# Upgrade

This article walks you through how to upgrade your Coder server.

> [!CAUTION]
> Prior to upgrading a production Coder deployment, take a database snapshot since
> Coder does not support rollbacks.

## Reinstall Coder to upgrade

<div class="tabs">

To upgrade your Coder server, simply reinstall Coder using your original method
of [install](../install).

## install.sh

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

## docker-compose

If you installed using `docker-compose`, run the below command to upgrade the
Coder container:

```shell
docker-compose pull coder && docker-compose up -d coder
```

## Kubernetes

See
[Upgrading Coder via Helm](../install/kubernetes.md#upgrading-coder-via-helm).

## Coder AMI on AWS

1. Run the Coder installation script on the host:

   ```shell
   curl -L https://coder.com/install.sh | sh
   ```

   The script will unpack the new `coder` binary version over the one currently
   installed.

1. If you're running Coder as a system service, you can restart it with `systemctl`:

   ```shell
   systemctl daemon-reload
   systemctl restart coder
   ```

## Windows

Download the latest Windows installer or binary from
[GitHub releases](https://github.com/coder/coder/releases/latest), or upgrade
from Winget.

```pwsh
winget install Coder.Coder
```

</div>
