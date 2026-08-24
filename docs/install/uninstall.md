<!-- markdownlint-disable MD024 -->
# Uninstall

This article walks you through how to uninstall your Coder server.

To uninstall your Coder server, delete the following directories.

## The Coder server binary and CLI

<div class="tabs">

## Linux

<div class="tabs">

## Debian, Ubuntu

```sh
sudo apt remove coder
```

## Fedora, CentOS, RHEL, SUSE

```sh
sudo yum remove coder
```

## Alpine

```sh
sudo apk del coder
```

</div>

If you installed Coder manually or used the install script on an unsupported
operating system, you can remove the binary directly:

```sh
sudo rm /usr/local/bin/coder
```

## macOS

```sh
brew uninstall coder
```

If you installed Coder manually, you can remove the binary directly:

```sh
sudo rm /usr/local/bin/coder
```

## Windows

```ps1
winget uninstall Coder.Coder
```

</div>

## Coder as a system service configuration

```sh
sudo rm /etc/coder.d/coder.env
```

## Coder settings, cache, and the optional built-in PostgreSQL database

There is a `postgres` directory within the `coderv2` directory that has the
database engine and database. If you want to reuse the database, consider not
performing the following step or copying the directory to another location.

<div class="tabs">

## Linux

```sh
rm -rf ~/.config/coderv2
rm -rf ~/.cache/coder
```

## macOS

```sh
rm -rf ~/Library/Application\ Support/coderv2
```

## Windows

```ps1
rmdir %AppData%\coderv2
```

</div>
