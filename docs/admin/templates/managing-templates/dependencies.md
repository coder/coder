# Template Dependencies

When creating Coder templates, it is unlikely that you will just be using
built-in providers. Part of Terraform's flexibility stems from its rich plugin
ecosystem, and it makes sense to take advantage of this.

That having been said, here are some recommendations to follow, based on the
[Terraform documentation](https://developer.hashicorp.com/terraform/tutorials/configuration-language/provider-versioning).

Following these recommendations will:

- **Prevent unexpected changes:** Your templates will use the same versions of
  Terraform providers each build. This will prevent issues related to changes in
  providers.
- **Improve build performance:** Coder caches provider versions on each build.
  If the same provider version can be re-used on subsequent builds, Coder will
  simply re-use the cached version if it is available.
- **Improve build reliability:** As some providers are hundreds of megabytes in
  size, interruptions in connectivity to the Terraform registry during a
  workspace build can result in a failed build. If Coder is able to re-use a
  cached provider version, the likelihood of this is greatly reduced.

## Lock your provider and module versions

If you add a Terraform provider to `required_providers` without specifying a
version requirement, Terraform will always fetch the latest version on each
invocation:

```terraform
terraform {
  required_providers {
    coder = {
      source = "coder/coder"
    }
    frobnicate = {
      source = "acme/frobnicate"
    }
  }
}
```

Any new releases of the `coder` or `frobnicate` providers will be picked up upon
the next time a workspace is built using this template. This may include
breaking changes.

To prevent this, add a
[version constraint](https://developer.hashicorp.com/terraform/language/expressions/version-constraints)
to each provider in the `required_providers` block:

```terraform
terraform {
  required_providers {
    coder = {
      source = "coder/coder"
      version = ">= 0.2, < 0.3"
    }
    frobnicate = {
      source = "acme/frobnicate"
      version = "~> 1.0.0"
    }
  }
}
```

In the above example, the `coder/coder` provider will be limited to all versions
above or equal to `0.2.0` and below `0.3.0`, while the `acme/frobnicate`
provider will be limited to all versions matching `1.0.x`.

The above also applies to Terraform modules. In the below example, the module
`razzledazzle` is locked to version `1.2.3`.

```terraform
module "razzledazzle" {
  source  = "registry.example.com/modules/razzle/dazzle"
  version = "1.2.3"
  foo     = "bar"
}
```

## Use a Dependency Lock File

Terraform allows creating a
[dependency lock file](https://developer.hashicorp.com/terraform/language/files/dependency-lock)
to track which provider versions were selected previously. This allows you to
ensure that the next workspace build uses the same provider versions as with the
last build.

To create a new Terraform lock file, run the
[`terraform init` command](https://developer.hashicorp.com/terraform/cli/commands/init)
inside a folder containing the Terraform source code for a given template.

This will create a new file named `.terraform.lock.hcl` in the current
directory. When you next run
[`coder templates push`](../../../reference/cli/templates_push.md), the lock
file will be stored alongside with the other template source code.

> [!NOTE]
> Terraform best practices also recommend checking in your
> `.terraform.lock.hcl` into Git or other VCS.

The next time a workspace is built from that template, Coder will make sure to
use the same versions of those providers as specified in the lock file.

If, at some point in future, you need to update the providers and versions you
specified within the version constraints of the template, run

```console
terraform init -upgrade
```

This will check each provider, check the newest satisfiable version based on the
version constraints you specified, and update the `.terraform.lock.hcl` with
those new versions. When you next run `coder templates push`, again, the updated
lock file will be stored and used to determine the provider versions to use for
subsequent workspace builds.
