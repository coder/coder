# Template Change Management

We recommend source-controlling your templates as you would other any code, and
automating the creation of new versions in CI/CD pipelines.

These pipelines will require tokens for your deployment. To cap token lifetime
on creation,
[configure Coder server to set a shorter max token lifetime](../../../reference/cli/server.md#--max-token-lifetime).

## coderd Terraform Provider

The
[coderd Terraform provider](https://registry.terraform.io/providers/coder/coderd/latest)
can be used to push new template versions, either manually, or in CI/CD
pipelines. To run the provider in a CI/CD pipeline, and to prevent drift, you'll
need to store the Terraform state
[remotely](https://developer.hashicorp.com/terraform/language/backend).

```tf
terraform {
  required_providers {
    coderd = {
      source = "coder/coderd"
    }
  }
  backend "gcs" {
    bucket = "example-bucket"
    prefix = "terraform/state"
  }
}

provider "coderd" {
  // Can be populated from environment variables
  url   = "https://coder.example.com"
  token = "****"
}

// Get the commit SHA of the configuration's git repository
variable "TFC_CONFIGURATION_VERSION_GIT_COMMIT_SHA" {
  type = string
}

resource "coderd_template" "kubernetes" {
  name = "kubernetes"
  description = "Develop in Kubernetes!"
  versions = [{
    directory = ".coder/templates/kubernetes"
    active    = true
    # Version name is optional
    name = var.TFC_CONFIGURATION_VERSION_GIT_COMMIT_SHA
    tf_vars = [{
      name  = "namespace"
      value = "default4"
    }]
  }]
  /* ... Additional template configuration */
}
```

For an example, see how we push our development image and template
[with GitHub actions](https://github.com/coder/coder/blob/main/.github/workflows/dogfood.yaml).

## Coder CLI

You can [install Coder](../../../install/cli.md) CLI to automate pushing new
template versions in CI/CD pipelines. For GitHub Actions, see our
[setup-coder](https://github.com/coder/setup-coder) action.

```console
# Install the Coder CLI
curl -L https://coder.com/install.sh | sh
# curl -L https://coder.com/install.sh | sh -s -- --version=0.x

# To create API tokens, use `coder tokens create`.
# If no `--lifetime` flag is passed during creation, the default token lifetime
# will be 30 days.
# These variables are consumed by Coder
export CODER_URL=https://coder.example.com
export CODER_SESSION_TOKEN=*****

# Template details
export CODER_TEMPLATE_NAME=kubernetes
export CODER_TEMPLATE_DIR=.coder/templates/kubernetes
export CODER_TEMPLATE_VERSION=$(git rev-parse --short HEAD)

# Push the new template version to Coder
coder templates push --yes $CODER_TEMPLATE_NAME \
    --directory $CODER_TEMPLATE_DIR \
    --name=$CODER_TEMPLATE_VERSION # Version name is optional
```

## Testing and Publishing Coder Templates in CI/CD

See our [testing templates](../../../tutorials/testing-templates.md) tutorial
for an example of how to test and publish Coder templates in a CI/CD pipeline.

### Next steps

- [Coder CLI Reference](../../../reference/cli/templates.md)
- [Coderd Terraform Provider Reference](https://registry.terraform.io/providers/coder/coderd/latest/docs)
- [Coderd API Reference](../../../reference/index.md)
