# Reusing template code

To reuse code across different Coder templates, such as common scripts or
resource definitions, we suggest using
[Terraform Modules](https://developer.hashicorp.com/terraform/language/modules).

You can store these modules externally from your Coder deployment, like in a git
repository or a Terraform registry. This example shows how to reference a module
from your template:

```hcl
data "coder_workspace" "me" {}

module "coder-base" {
  source = "github.com/my-organization/coder-base"

  # Modules take in variables and can provision infrastructure
  vpc_name            = "devex-3"
  subnet_tags         = { "name": data.coder_workspace.me.name }
  code_server_version = 4.14.1
}

resource "coder_agent" "dev" {
  # Modules can provide outputs, such as helper scripts
  startup_script=<<EOF
  #!/bin/sh
  ${module.coder-base.code_server_install_command}
  EOF
}
```

Learn more about
[creating modules](https://developer.hashicorp.com/terraform/language/modules)
and
[module sources](https://developer.hashicorp.com/terraform/language/modules/sources)
in the Terraform documentation.

## Git authentication

If you are importing a module from a private git repository, the Coder server or
[provisioner](../admin/provisioners.md) needs git credentials. Since this token
will only be used for cloning your repositories with modules, it is best to
create a token with access limited to the repository and no extra permissions.
In GitHub, you can generate a
[fine-grained token](https://docs.github.com/en/rest/overview/permissions-required-for-fine-grained-personal-access-tokens?apiVersion=2022-11-28)
with read only access to the necessary repos.

If you are running Coder on a VM, make sure that you have `git` installed and
the `coder` user has access to the following files:

```shell
# /home/coder/.gitconfig
[credential]
  helper = store
```

```shell
# /home/coder/.git-credentials

# GitHub example:
https://your-github-username:your-github-pat@github.com
```

If you are running Coder on Docker or Kubernetes, `git` is pre-installed in the
Coder image. However, you still need to mount credentials. This can be done via
a Docker volume mount or Kubernetes secrets.

### Passing git credentials in Kubernetes

First, create a `.gitconfig` and `.git-credentials` file on your local machine.
You might want to do this in a temporary directory to avoid conflicting with
your own git credentials.

Next, create the secret in Kubernetes. Be sure to do this in the same namespace
that Coder is installed in.

```shell
export NAMESPACE=coder
kubectl apply -f - <<EOF
apiVersion: v1
kind: Secret
metadata:
  name: git-secrets
  namespace: $NAMESPACE
type: Opaque
data:
  .gitconfig: $(cat .gitconfig | base64 | tr -d '\n')
  .git-credentials: $(cat .git-credentials | base64 | tr -d '\n')
EOF
```

Then, modify Coder's Helm values to mount the secret.

```yaml
coder:
  volumes:
    - name: git-secrets
      secret:
        secretName: git-secrets
  volumeMounts:
    - name: git-secrets
      mountPath: "/home/coder/.gitconfig"
      subPath: .gitconfig
      readOnly: true
    - name: git-secrets
      mountPath: "/home/coder/.git-credentials"
      subPath: .git-credentials
      readOnly: true
```

## Artifactory

JFrog Artifactory can serve as a Terraform module registry, allowing you to
simplify a Coder-stored template to a `module` block and input variables.

With this approach, you can:

- Easily share templates across multiple Coder instances
- Store templates far larger than the 1MB limit of Coder's template storage
- Apply JFrog platform security policies to your templates

### Basic Scaffolding

For example, a template with:

```hcl
module "frontend" {
  source = "cdr.jfrog.io/tf__main/frontend/docker"
}
```

References the `frontend` module in the `main` namespace of the `tf` repository.
Remember to replace `cdr.jfrog.io` with your Artifactory instance URL.

You can upload the underlying module to Artifactory with:

```shell
# one-time setup commands
# run this on the coder server (or external provisioners, if you have them)
terraform login cdr.jfrog.io; jf tfc --global

# jf tf p assumes the module name is the same as the current directory name.
jf tf p --namespace=main --provider=docker --tag=v0.0.1
```

### Example template

We have an example template
[here](https://github.com/coder/coder/tree/main/examples/jfrog/remote) that uses
our [JFrog Docker](../platforms/jfrog.md) template as the underlying module.

### Next up

Learn more about

- JFrog's Terraform Registry support
  [here](https://jfrog.com/help/r/jfrog-artifactory-documentation/terraform-registry).
- Configuring the JFrog toolchain inside a workspace
  [here](../platforms/jfrog.md).
- Coder Module Registry [here](https://registry.coder.com/modules)
