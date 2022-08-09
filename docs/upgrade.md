# Upgrade

This article walks you through how to upgrade your Coder server.

## Upgrading Coder

To upgrade your Coder server, simply re-run the `install.sh` script on the Coder
server host:

```console
curl -L https://coder.com/install.sh | sh
```

The script will unpack the new `coder` binary version over the one currently installed.
Once this is run, you can restart Coder with the following command (if running
it as a system service):

```console
systemctl stop coder && systemctl start coder
```
