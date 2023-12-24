# JFrog

Use Coder and JFrog together to secure your development environments without
disturbing your developers' existing workflows.

This guide will demonstrate how to use JFrog Artifactory as a package registry
within a workspace. We'll use Docker as the underlying compute. But, these
concepts apply to any compute platform.

The full example template can be found
[here](https://github.com/coder/coder/tree/main/examples/jfrog/docker).

## Requirements

- A JFrog Artifactory instance
- An admin-level access token for Artifactory
- 1:1 mapping of users in Coder to users in Artifactory by email address and
  username
- Repositories configured in Artifactory for each package manager you want to
  use

<blockquote class="info">
The admin-level access token is used to provision user tokens and is never exposed to
developers or stored in workspaces.
</blockquote>

<blockquote class="info">
You can skip the whole page and use [JFrog module](https://registry.coder.com/modules/jfrog-token) for easy JFrog Artifactory integration.
</blockquote>

## Provisioner Authentication

The most straight-forward way to authenticate your template with Artifactory is
by using
[Terraform-managed variables](https://coder.com/docs/v2/latest/templates/parameters#terraform-template-wide-variables).

See the following example:

```hcl
terraform {
  required_providers {
    coder = {
      source  = "coder/coder"
      version = "~> 0.11.1"
    }
    docker = {
      source  = "kreuzwerker/docker"
      version = "~> 3.0.1"
    }
    artifactory = {
      source  = "registry.terraform.io/jfrog/artifactory"
      version = "~> 8.4.0"
    }
  }
}

variable "jfrog_host" {
  type        = string
  description = "JFrog instance hostname. e.g. YYY.jfrog.io"
}

variable "artifactory_access_token" {
  type        = string
  description = "The admin-level access token to use for JFrog."
}

# Configure the Artifactory provider
provider "artifactory" {
  url           = "https://${var.jfrog_host}/artifactory"
  access_token  = "${var.artifactory_access_token}"
}

resource "artifactory_scoped_token" "me" {
  # This is hacky, but on terraform plan the data source gives empty strings,
  # which fails validation.
  username = length(local.artifactory_username) > 0 ? local.artifactory_username : "plan"
}
```

When pushing the template, you can pass in the variables using the `--var` flag:

```shell
coder templates push --var 'jfrog_host=YYY.jfrog.io' --var 'artifactory_access_token=XXX'
```

## Installing JFrog CLI

`jf` is the JFrog CLI. It can do many things across the JFrog platform, but
we'll focus on its ability to configure package managers, as that's the relevant
functionality for most developers.

Most users should be able to install `jf` by running the following command:

```shell
curl -fL https://install-cli.jfrog.io | sh
```

Other methods are listed [here](https://jfrog.com/getcli/).

In our Docker-based example, we install `jf` by adding these lines to our
`Dockerfile`:

```Dockerfile
RUN curl -fL https://install-cli.jfrog.io | sh && chmod 755 $(which jf)
```

## Configuring Coder workspace to use JFrog Artifactory repositories

Create a `locals` block to store the Artifactory repository keys for each
package manager you want to use in your workspace. For example, if you want to
use artifactory repositories with keys `npm`, `pypi`, and `go`, you can create a
`locals` block like this:

```hcl
locals {
  artifactory_repository_keys = {
    npm    = "npm"
    python = "pypi"
    go     = "go"
  }
}
```

To automatically configure `jf` CLI and Artifactory repositories for each user,
add the following lines to your `startup_script` in the `coder_agent` block:

```hcl
resource "coder_agent" "main" {
  arch                   = data.coder_provisioner.me.arch
  os                     = "linux"
  startup_script_timeout = 180
  startup_script         = <<-EOT
    set -e

    # install and start code-server
    curl -fsSL https://code-server.dev/install.sh | sh -s -- --method=standalone --prefix=/tmp/code-server --version 4.11.0
    /tmp/code-server/bin/code-server --auth none --port 13337 >/tmp/code-server.log 2>&1 &

    # The jf CLI checks $CI when determining whether to use interactive
    # flows.
    export CI=true

    jf c rm 0 || true
    echo ${artifactory_scoped_token.me.access_token} | \
      jf c add --access-token-stdin --url https://${var.jfrog_host} 0

    # Configure the `npm` CLI to use the Artifactory "npm" repository.
    cat << EOF > ~/.npmrc
    email = ${data.coder_workspace.me.owner_email}
    registry = https://${var.jfrog_host}/artifactory/api/npm/${local.artifactory_repository_keys["npm"]}
    EOF
    jf rt curl /api/npm/auth >> .npmrc

    # Configure the `pip` to use the Artifactory "python" repository.
    mkdir -p ~/.pip
    cat << EOF > ~/.pip/pip.conf
    [global]
    index-url = https://${local.artifactory_username}:${artifactory_scoped_token.me.access_token}@${var.jfrog_host}/artifactory/api/pypi/${local.artifactory_repository_keys["python"]}/simple
    EOF

  EOT
  # Set GOPROXY to use the Artifactory "go" repository.
  env = {
    GOPROXY : "https://${local.artifactory_username}:${artifactory_scoped_token.me.access_token}@${var.jfrog_host}/artifactory/api/go/${local.artifactory_repository_keys["go"]}"
  }
}
```

You can verify that `jf` is configured correctly in your workspace by running
`jf c show`. It should display output like:

```text
coder@jf:~$ jf c show
Server ID:                      0
JFrog Platform URL:             https://YYY.jfrog.io/
Artifactory URL:                https://YYY.jfrog.io/artifactory/
Distribution URL:               https://YYY.jfrog.io/distribution/
Xray URL:                       https://YYY.jfrog.io/xray/
Mission Control URL:            https://YYY.jfrog.io/mc/
Pipelines URL:                  https://YYY.jfrog.io/pipelines/
User:                           ammar@....com
Access token:                   ...
Default:                        true
```

## Installing the JFrog VS Code Extension

You can install the JFrog VS Code extension into workspaces by inserting the
following lines into your `startup_script`:

```shell
# Install the JFrog VS Code extension.
# Find the latest version number at
# https://open-vsx.org/extension/JFrog/jfrog-vscode-extension.
JFROG_EXT_VERSION=2.4.1
curl -o /tmp/jfrog.vsix -L "https://open-vsx.org/api/JFrog/jfrog-vscode-extension/$JFROG_EXT_VERSION/file/JFrog.jfrog-vscode-extension-$JFROG_EXT_VERSION.vsix"
/tmp/code-server/bin/code-server --install-extension /tmp/jfrog.vsix
```

Note that this method will only work if your developers use code-server.

## Configuring npm

Add the following line to your `startup_script` to configure `npm` to use
Artifactory:

```shell
    # Configure the `npm` CLI to use the Artifactory "npm" registry.
    cat << EOF > ~/.npmrc
    email = ${data.coder_workspace.me.owner_email}
    registry = https://${var.jfrog_host}/artifactory/api/npm/npm/
    EOF
    jf rt curl /api/npm/auth >> .npmrc
```

Now, your developers can run `npm install`, `npm audit`, etc. and transparently
use Artifactory as the package registry. You can verify that `npm` is configured
correctly by running `npm install --loglevel=http react` and checking that npm
is only hitting your Artifactory URL.

## Configuring pip

Add the following lines to your `startup_script` to configure `pip` to use
Artifactory:

```shell
    mkdir -p ~/.pip
    cat << EOF > ~/.pip/pip.conf
    [global]
    index-url = https://${data.coder_workspace.me.owner}:${artifactory_scoped_token.me.access_token}@${var.jfrog_host}/artifactory/api/pypi/pypi/simple
    EOF
```

Now, your developers can run `pip install` and transparently use Artifactory as
the package registry. You can verify that `pip` is configured correctly by
running `pip install --verbose requests` and checking that pip is only hitting
your Artifactory URL.

## Configuring Go

Add the following environment variable to your `coder_agent` block to configure
`go` to use Artifactory:

```hcl
  env = {
    GOPROXY : "https://${data.coder_workspace.me.owner}:${artifactory_scoped_token.me.access_token}@${var.jfrog_host}/artifactory/api/go/go"
  }
```

You can apply the same concepts to Docker, Maven, and other package managers
supported by Artifactory. See the
[JFrog documentation](https://jfrog.com/help/r/jfrog-artifactory-documentation/package-management)
for more information.

## More reading

- See the full example template
  [here](https://github.com/coder/coder/tree/main/examples/jfrog/docker).
- To serve extensions from your own VS Code Marketplace, check out
  [code-marketplace](https://github.com/coder/code-marketplace#artifactory-storage).
- To store templates in Artifactory, check out our
  [Artifactory modules](../templates/modules.md#artifactory) docs.
