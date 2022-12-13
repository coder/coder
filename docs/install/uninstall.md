# Uninstall

This article walks you through how to uninstall your Coder server.

To uninstall your Coder server, delete the following directories.

## Cached Coder releases

```console
rm -rf ~/.cache/coder
```

## The Coder server binary and CLI

Debian, Ubuntu:

```console
sudo apt remove coder
```

Fedora, CentOS, RHEL, SUSE:

```console
sudo yum remove coder
```

Alpine:

```console
sudo apk del coder
```

If you installed Coder manually or used the install script on an unsupported operating system, you can remove the binary directly:

```console
sudo rm /usr/local/bin/coder
```

## Coder as a system service configuration

```console
sudo rm /etc/coder.d/coder.env
```

## Coder settings and the optional built-in PostgreSQL database

> There is a `postgres` directory within the `coderv2` directory that has the
> database engine and database. If you want to reuse the database, consider
> not performing the following step or copying the directory to another
> location.

### macOS

```console
rm -rf ~/Library/Application\ Support/coderv2
```

### Linux

```console
rm -rf ~/.config/coderv2
```

### Windows

```console
C:\Users\USER\AppData\Roaming\coderv2
```
