# Uninstall

This article walks you through how to uninstall your Coder server.

<blockquote class="danger">
  <p>
  Uninstalling Coder with the built-in PostgreSQL database will delete the database engine and database. Consider backing up the database directory if you would like to reuse it with a future installation. This does not apply if you have installed your own PostgreSQL database instance.
  </p>
</blockquote>

To uninstall your Coder server, delete the following directories.

## Cached Coder releases

```console
rm -rf ~/.cache/coder
```

## The Coder server binary and CLI

Debian, Ubuntu:

```sh
sudo apt remove coder
```

Fedora, CentOS, RHEL, SUSE:

```sh
sudo yum remove coder
```

Alpine:

```sh
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

## Up Next

- [Learn how to configure Coder](./configure.md).
