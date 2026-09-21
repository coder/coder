---
title: Upgrade
---

This article describes how to upgrade your Coder server.

> [!CAUTION]
> Coder does not support rolling or live upgrades while users have active
> sessions, workspace connections, or running workspace builds, and Coder
> does not support rollbacks. Always upgrade during a scheduled maintenance
> window, and take a database snapshot beforehand.

For the full, prescriptive procedure for safely upgrading a production
deployment (announcing a maintenance window, confirming no active user
activity, backing up your database, and verifying health after the upgrade),
see [Upgrading Best Practices](./upgrade-best-practices.md).

## Reinstall Coder to upgrade

To upgrade your Coder server, reinstall Coder using your original method
of [install](../install/index.md).

### Coder install script

1. If you installed Coder using the `install.sh` script, re-run the below command
   on the host:

   ```sh
   curl -L https://coder.com/install.sh | sh
   ```

1. If you're running Coder as a system service, you can restart it with `systemctl`:

   ```sh
   systemctl daemon-reload
   systemctl restart coder
   ```

### Other upgrade methods

<div class="tabs">

### docker-compose

If you installed using `docker-compose`, run the below command to upgrade the
Coder container:

```sh
docker-compose pull coder && docker-compose up -d coder
```

### Kubernetes

See
[Upgrade Coder via Helm](../install/kubernetes.md#upgrade-coder-via-helm).

### Coder AMI on AWS

1. Run the Coder installation script on the host:

   ```sh
   curl -L https://coder.com/install.sh | sh
   ```

   The script will unpack the new `coder` binary version over the one currently
   installed.

1. Restart the Coder system process with `systemctl`:

   ```sh
   systemctl daemon-reload
   systemctl restart coder
   ```

### Windows

Download the latest Windows installer or binary from
[GitHub releases](https://github.com/coder/coder/releases/latest), or upgrade
from Winget.

```ps1
winget install Coder.Coder
```

</div>
