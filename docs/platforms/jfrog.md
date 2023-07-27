# JFrog

Use Coder and JFrog together to secure your development environments without disturbing your developers' existing workflows.

This guide will demonstrate how to use JFrog Artifactory as a package registry
within a workspace. We'll use Docker as the underlying compute. But, these concepts apply to any compute platform.

The full example template can be found [here](https://github.com/coder/coder/tree/main/examples/templates/jfrog-docker).

## Requirements

- A JFrog Artifactory instance
- An admin-level access token for Artifactory
- 1:1 mapping of users in Coder to users in Artifactory by email address
- An npm repository in Artifactory named "npm"

<blockquote class="info">
The admin-level access token is used to provision user tokens and is never exposed to
developers or stored in workspaces.
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
      version = "6.22.3"
    }
  }
}

variable "jfrog_url" {
  type        = string
  description = "The URL of the JFrog instance."
}

variable "artifactory_access_token" {
  type        = string
  description = "The admin-level access token to use for JFrog."
}

# Configure the Artifactory provider
provider "artifactory" {
  url           = "${var.jfrog_url}/artifactory"
  access_token  = "${var.artifactory_access_token}"
}
```

When pushing the template, you can pass in the variables using the `-V` flag:

```sh
coder templates push --var 'jfrog_url=https://YYY.jfrog.io' --var 'artifactory_access_token=XXX'
```

## Installing JFrog CLI

`jf` is the JFrog CLI. It can do many things across the JFrog platform, but
we'll focus on its ability to configure package managers, as that's the relevant
functionality for most developers.

The generic method of installing the JFrog CLI is the following command:

```sh
curl -fL https://install-cli.jfrog.io | sh
```

Other methods are listed [here](https://jfrog.com/getcli/).

In our Docker-based example, we install `jf` by adding these lines to our `Dockerfile`:

```Dockerfile
RUN curl -fL https://install-cli.jfrog.io | sh && chmod 755 $(which jf)
```

and use this `coder_agent` block:

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
    echo ${artifactory_access_token.me.access_token} | \
      jf c add --access-token-stdin --url ${var.jfrog_url} 0
  EOT
}
```

You can verify that `jf` is configured correctly in your workspace by
running `jf c show`. It should display output like:

```text
coder@jf:~$ jf c show
Server ID:                      0
JFrog Platform URL:             https://cdr.jfrog.io/
Artifactory URL:                https://cdr.jfrog.io/artifactory/
Distribution URL:               https://cdr.jfrog.io/distribution/
Xray URL:                       https://cdr.jfrog.io/xray/
Mission Control URL:            https://cdr.jfrog.io/mc/
Pipelines URL:                  https://cdr.jfrog.io/pipelines/
User:                           ammar@....com
Access token:                   ...
Default:                        true
```

## Installing the JFrog VS Code Extension

You can install the JFrog VS Code extension into workspaces automatically
by inserting the following lines into your `startup_script`:

```sh
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

```sh
    # Configure the `npm` CLI to use the Artifactory "npm" registry.
    cat << EOF > ~/.npmrc
    email = ${data.coder_workspace.me.owner_email}
    registry=${var.jfrog_url}/artifactory/api/npm/npm/
    EOF
    jf rt curl /api/npm/auth >> .npmrc
```

Now, your developers can run `npm install`, `npm audit`, etc. and transparently
use Artifactory as the package registry. You can verify that `npm` is configured
correctly by running `npm install --loglevel=http react` and checking that
npm is only hitting your Artifactory URL.

You can apply the same concepts to Docker, Go, Maven, and other package managers
supported by Artifactory.

## More reading

- See the full example template [here](https://github.com/coder/coder/tree/main/examples/templates/jfrog-docker).
- To serve extensions from your own VS Code Marketplace, check out [code-marketplace](https://github.com/coder/code-marketplace#artifactory-storage).
