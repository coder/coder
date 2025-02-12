# Setting up a Coder deployment

For day-zero Coder users, we recommend following this guide to set up a local
Coder deployment from our
[open source repository](https://github.com/coder/coder).

We'll use [Docker](https://docs.docker.com/engine) to manage the compute for a
slim deployment to experiment with [workspaces](../user-guides/index.md) and
[templates](../admin/templates/index.md).

Docker is not necessary for every Coder deployment and is only used here for
simplicity.

## Install Coder daemon

First, install [Docker](https://docs.docker.com/engine/install/) locally.

> If you already have the Coder binary installed, restart it after installing
> Docker.

<div class="tabs">

## Linux/macOS

Our install script is the fastest way to install Coder on Linux/macOS:

```sh
curl -L https://coder.com/install.sh | sh
```

## Windows

> **Important:** If you plan to use the built-in PostgreSQL database, you will
> need to ensure that the
> [Visual C++ Runtime](https://learn.microsoft.com/en-US/cpp/windows/latest-supported-vc-redist#latest-microsoft-visual-c-redistributable-version)
> is installed.

You can use the
[`winget`](https://learn.microsoft.com/en-us/windows/package-manager/winget/#use-winget)
package manager to install Coder:

```powershell
winget install Coder.Coder
```

</div>

## Start the server

To start or restart the Coder deployment, use the following command:

```shell
coder server
```

The output will provide you with an access URL to create your first
administrator account.

![Coder login screen](../images/start/setup-page.png)

Once you've signed in, you'll be brought to an empty workspaces page, which
we'll soon populate with your first development environments.

## Next steps

TODO: Add link to next page.
