The easiest way to install Coder is to use our
[install script](https://github.com/coder/coder/blob/main/install.sh) for Linux
and macOS.

To install, run:

```shell
# This will automatically use supported package managers when available
curl -fsSL https://coder.com/install.sh | sh
```

To install without using a system package manager:

```shell
curl -fsSL https://coder.com/install.sh | sh -s -- --method standalone
```

You can preview what occurs during the install process:

```shell
curl -fsSL https://coder.com/install.sh | sh -s -- --dry-run
```

You can modify the installation process by including flags. Run the help command
for reference:

```shell
curl -fsSL https://coder.com/install.sh | sh -s -- --help
```

After installing, use the in-terminal instructions to start the Coder server
manually via `coder server` or as a system package.

By default, the Coder server runs on `http://127.0.0.1:3000` and uses a
[public tunnel](../admin/configure.md#tunnel) for workspace connections.

## Next steps

- [Configuring Coder](../admin/configure.md)
- [Templates](../templates/index.md)
