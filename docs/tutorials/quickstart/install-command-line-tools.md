# Install your own command-line tools

Now that you've finished [Launch your first workspace](./launch-workspace.md),
you can add your favorite command-line tools to every workspace.

The Quickstart template installs system languages through the **Programming Languages** parameter,
but it doesn't carry the small command-line tools you may often use,
such as [`bat`](https://github.com/sharkdp/bat) or [`ripgrep`](https://github.com/BurntSushi/ripgrep).
You can install those yourself with a package manager like [Homebrew](https://brew.sh/) or [mise](https://mise.jdx.dev/).

In this guide, you install both Homebrew and mise,
install a tool with each,
and learn which installs survive a workspace restart and why.

> [!NOTE]
> This guide works inside a running workspace from the Quickstart template.
> It doesn't edit the template, though making Homebrew tools persist does, as you'll see at the end.

## What you'll do

- ✅ Install command-line tools with [Homebrew](https://brew.sh/) and [mise](https://mise.jdx.dev/) into your workspace.
- ✅ Restart the workspace and see which tools persist.
- ✅ Learn why one persists and the other doesn't, and how to make either one stay.

## What persists in a workspace

A Quickstart workspace keeps your home directory, `/home/coder`, on a persistent volume.
Everything outside `/home/coder` comes from the workspace image,
and Coder rebuilds it from that image every time the workspace starts.

A tool survives a restart only when both of these are true:

- The tool installs into `/home/coder`.
- Your shell finds the tool through a file in `/home/coder`, such as `.bashrc`.

You'll install tools two ways and then restart to see this rule decide which ones stay.

## Step 1: Install Homebrew and mise

Open a terminal in your workspace.

Install [Homebrew](https://brew.sh/) with its setup script:

```sh
NONINTERACTIVE=1 /bin/bash -c "$(curl -fsSL https://raw.githubusercontent.com/Homebrew/install/HEAD/install.sh)"
```

Homebrew installs to `/home/linuxbrew/.linuxbrew`.
Add it to your shell so the `brew` command is available:

```sh
echo 'eval "$(/home/linuxbrew/.linuxbrew/bin/brew shellenv)"' >> ~/.bashrc
eval "$(/home/linuxbrew/.linuxbrew/bin/brew shellenv)"
```

Install [mise](https://mise.jdx.dev/) with its setup script:

```sh
curl -fsSL https://mise.run | sh
```

mise installs to `~/.local/bin/mise`, inside your home directory.
Activate it in every new shell:

```sh
echo 'eval "$(~/.local/bin/mise activate bash)"' >> ~/.bashrc
```

Open a new terminal so both changes take effect,
then confirm each manager runs:

```sh
brew --version
mise --version
```

> [!NOTE]
> If you manage `~/.bashrc` with [dotfiles](./personalize-with-dotfiles.md),
> add the `brew shellenv` and `mise activate` lines to the `.bashrc` in your dotfiles repository instead,
> so applying your dotfiles doesn't overwrite them.

## Step 2: Install a tool with each manager

Install [`ripgrep`](https://github.com/BurntSushi/ripgrep) with Homebrew:

```sh
brew install ripgrep
```

Install [`bat`](https://github.com/sharkdp/bat) with mise:

```sh
mise use -g bat
```

Confirm both tools run:

```sh
rg --version
bat --version
```

Both work. So far, the two package managers look interchangeable.

## Step 3: Restart the workspace and compare

Restart the workspace from the Coder dashboard.
The restart rebuilds the container from the image and keeps only your home directory.

Open a terminal and run both commands again:

```sh
bat --version
rg --version
```

This time the results differ.
`bat` reports its version, because mise installed it under `/home/coder`, which persists.
`rg` is gone, and so is `brew` itself:

```text
rg: command not found
brew: command not found
```

Homebrew installed `ripgrep` to `/home/linuxbrew`, outside `/home/coder`,
so the rebuild discarded both Homebrew and every formula you installed with it.

## What just happened

The two package managers behaved differently for one reason: where each one installs.

- mise installs into `~/.local/share/mise`, inside your home directory, and activates from `~/.bashrc`. Both are in `/home/coder`, so its tools persist with no template change.
- Homebrew installs to `/home/linuxbrew`, outside `/home/coder`, so its tools are discarded on every restart.

To keep a tool, choose the approach that matches who needs it:

- For a tool that's yours alone, install it with mise. It persists through restarts with no further setup.
- To make Homebrew persist, update the template to mount its prefix, `/home/linuxbrew`, on a persistent volume, the way the Coder dogfood template does. This is a template change, so it affects everyone who uses the template.
- For a tool everyone needs preinstalled, add it to the startup script with `apt-get`, as in [Add a programming language](./add-a-language.md), or bake it into the workspace image.

The rule underneath all three: a tool persists when it lives in a part of the workspace that persists.
Refer to [Resource persistence](../../admin/templates/extending-templates/resource-persistence.md) for how Coder decides what survives a restart.

## What's next?

Now that you can install your own tools, [personalize your workspace with dotfiles](./personalize-with-dotfiles.md).

## Learn more

- [Homebrew documentation](https://brew.sh/) for the package manager
- [mise documentation](https://mise.jdx.dev/) for the version manager
- [Resource persistence](../../admin/templates/extending-templates/resource-persistence.md) in the Coder documentation
- [Dotfiles](../../user-guides/workspace-dotfiles.md) in the Coder documentation
