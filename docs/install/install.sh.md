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

## `PATH` conflicts

It's possible to end up in situations where you have multiple `coder` binaries
in your `PATH`, and your system may use a version that you don't intend. Your
`PATH` is a variable that tells your shell where to look for programs to run.

You can check where all of the versions are by running `which -a coder`.

For example, a common conflict on macOS might be between a version installed by
Homebrew, and a version installed manually to the /usr/local/bin directory.

```console
$ which -a coder
/usr/local/bin/coder
/opt/homebrew/bin/coder
```

Whichever binary comes first in this list will be used when running `coder`
commands.

### Reordering your `PATH`

If you use bash or zsh, you can update your `PATH` like this:

```shell
# You might want to add this line to the end of your ~/.bashrc or ~/.zshrc file!
export PATH="/opt/homebrew/bin:$PATH"
```

If you use fish, you can update your `PATH` like this:

```shell
# You might want to add this line to the end of your ~/.config/fish/config.fish file!
fish_add_path "/opt/homebrew/bin"
```

> ℹ If you ran install.sh with a `--prefix` flag, you can replace
> `/opt/homebrew` with whatever value you used there. Make sure to leave the
> `/bin` at the end!

Now we can observe that the order has changed:

```console
$ which -a coder
/opt/homebrew/bin/coder
/usr/local/bin/coder
```

### Removing unneeded binaries

If you want to uninstall a version of `coder` that you installed with a package
manager, you can run whichever one of these commands applies:

```shell
# On macOS, with Homebrew installed
brew uninstall coder
```

```shell
# On Debian/Ubuntu based systems
sudo dpkg -r coder
```

```shell
# On Fedora/RHEL-like systems
sudo rpm -e coder
```

```shell
# On Alpine
sudo apk del coder
```

If the conflicting binary is not installed by your system package manager, you
can just delete it.

```shell
# You might not need `sudo`, depending on the location
sudo rm /usr/local/bin/coder
```

## Next steps

- [Configuring Coder](../admin/configure.md)
- [Templates](../templates/index.md)
