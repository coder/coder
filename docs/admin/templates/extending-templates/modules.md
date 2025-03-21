# Reusing template code

To reuse code across different Coder templates, such as common scripts or
resource definitions, we suggest using
[Terraform Modules](https://developer.hashicorp.com/terraform/language/modules).

You can store these modules externally from your Coder deployment, like in a git
repository or a Terraform registry. This example shows how to reference a module
from your template:

```tf
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

## Coder modules

Coder publishes plenty of modules that can be used to simplify some common tasks
across templates. Some of the modules we publish are,

1. [`code-server`](https://registry.coder.com/modules/code-server) and
   [`vscode-web`](https://registry.coder.com/modules/vscode-web)
2. [`git-clone`](https://registry.coder.com/modules/git-clone)
3. [`dotfiles`](https://registry.coder.com/modules/dotfiles)
4. [`jetbrains-gateway`](https://registry.coder.com/modules/jetbrains-gateway)
5. [`jfrog-oauth`](https://registry.coder.com/modules/jfrog-oauth) and
   [`jfrog-token`](https://registry.coder.com/modules/jfrog-token)
6. [`vault-github`](https://registry.coder.com/modules/vault-github)

For a full list of available modules please check
[Coder module registry](https://registry.coder.com/modules).

## Offline installations

In offline and restricted deploymnets, there are 2 ways to fetch modules.

1. Artifactory
2. Private git repository

### Artifactory

Air gapped users can clone the [coder/modules](https://github.com/coder/modules)
repo and publish a
[local terraform module repository](https://jfrog.com/help/r/jfrog-artifactory-documentation/set-up-a-terraform-module/provider-registry)
to resolve modules via [Artifactory](https://jfrog.com/artifactory/).

1. Create a local-terraform-repository with name `coder-modules-local`
2. Create a virtual repository with name `tf`
3. Follow the below instructions to publish coder modules to Artifactory

   ```shell
   git clone https://github.com/coder/modules
   cd modules
   jf tfc
   jf tf p --namespace="coder" --provider="coder" --tag="1.0.0"
   ```

4. Generate a token with access to the `tf` repo and set an `ENV` variable
   `TF_TOKEN_example.jfrog.io="XXXXXXXXXXXXXXX"` on the Coder provisioner.
5. Create a file `.terraformrc` with following content and mount at
   `/home/coder/.terraformrc` within the Coder provisioner.

   ```tf
   provider_installation {
     direct {
         exclude = ["registry.terraform.io/*/*"]
     }
     network_mirror {
         url = "https://example.jfrog.io/artifactory/api/terraform/tf/providers/"
     }
   }
   ```

6. Update module source as:

   ```tf
   module "module-name" {
     source = "https://example.jfrog.io/tf__coder/module-name/coder"
     version = "1.0.0"
     agent_id = coder_agent.example.id
     ...
   }
   ```

   Replace `example.jfrog.io` with your Artifactory URL

Based on the instructions
[here](https://jfrog.com/blog/tour-terraform-registries-in-artifactory/).

#### Example template

We have an example template
[here](https://github.com/coder/coder/blob/main/examples/jfrog/remote/main.tf)
that uses our
[JFrog Docker](https://github.com/coder/coder/blob/main/examples/jfrog/docker/main.tf)
template as the underlying module.

### Private git repository

If you are importing a module from a private git repository, the Coder server or
[provisioner](../../provisioners.md) needs git credentials. Since this token
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

#### Passing git credentials in Kubernetes

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

### Next steps

- JFrog's
  [Terraform Registry support](https://jfrog.com/help/r/jfrog-artifactory-documentation/terraform-registry)
- [Configuring the JFrog toolchain inside a workspace](../../integrations/jfrog-artifactory.md)
- [Coder Module Registry](https://registry.coder.com/modules)
