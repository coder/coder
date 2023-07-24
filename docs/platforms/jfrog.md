# JFrog

Coder and JFrog work together to provide seamless security and compliance for
your development environments. With Coder, you can automatically authenticate
every workspace to use Artifactory as a package registry.

In this page, we'll show you how to integrate both products using a Docker template
as an example. But, these concepts apply to any compute platform. The full example template can be found [here](https://github.com/coder/coder/tree/main/examples/jfrog-docker).

## Requirements

- A JFrog Artifactory instance
- 1:1 mapping of users in Coder to users in Artifactory, with the same email

## Provisioner Authentication

The most straight-forward way to authenticate your template with artifactory is
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
  description = "The access token to use for JFrog."
}


# Configure the Artifactory provider
provider "artifactory" {
  url           = "${var.jfrog_url}/artifactory"
  access_token  = "${var.artifactory_access_token}"
}
```

When pushing the template, you can pass in the variables using the `--variable` flag:

```sh
coder templates push --variable 'jfrog_url=https://YYY.jfrog.io' --variable 'artifactory_access_token=XXX'
```

## Installing jf

`jf` is the JFrog CLI. Every workspace must have `jf` installed to use
Artifactory.

The generic method of installing the JFrog CLI is the following command:

```sh
curl -fL https://install-cli.jfrog.io | sh
```

Other methods are listed [here](https://jfrog.com/help/r/jfrog-cli/download-and-installation).

In our Docker-based example, we install `jf` by adding these lines to our `Dockerfile`:

```Dockerfile
RUN curl -fL https://install-cli.jfrog.io | sh
RUN chmod 755 $(which jf)
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
running `jf c show`. It should present output like:

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

## Configuring npm

Add the following line to your `startup_script` to configure `npm` to use
Artifactory:

```sh
    # Configure the `npm` CLI to use the Artifactory "npm" registry.
    cat << EOF > ~/.npmrc
    _auth = ${artifactory_access_token.me.access_token}
    email = ${data.coder_workspace.me.owner_email}
    always-auth = true
    registry=${var.jfrog_url}/artifactory/api/npm/npm/
    EOF
```

Now, your developers can run `npm install`, `npm audit`, etc. and transparently
use Artifactory as the package registry.

You can apply the same concepts to Docker, Go, Maven, and other package managers
supported by Artifactory.

## Next steps

* If you'd like to serve extensions from your own VS Code Marketplace, check out
[`code-marketplace`](https://github.com/coder/code-marketplace#artifactory-storage).
* See the full example template [here](https://github.com/coder/coder/tree/main/examples/jfrog-docker).
