# Template inheritance

In instances where you want to reuse code across different Coder templates, such as common scripts or resource definitions, we suggest using [Terraform Modules](https://developer.hashicorp.com/terraform/language/modules).

These modules can be stored externally from Coder, like in a Git repository or a Terraform registry. Below is an example of how to reference a module in your template:

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

> Learn more about [creating modules](https://developer.hashicorp.com/terraform/language/modules) and [module sources](https://developer.hashicorp.com/terraform/language/modules/sources) in the Terraform documentation.

## Git authentication

If you are importing a module from a private git repository, the Coder server [or provisioner](../admin/provisioners.md) needs git credentials. Since this token will only be used for cloning your repositories with modules, it is best to create a token with limited access to repositories and no extra permissions. In GitHub, you can generate a [fine-grained token](https://docs.github.com/en/rest/overview/permissions-required-for-fine-grained-personal-access-tokens?apiVersion=2022-11-28) with read only access to repos.

If you are running Coder on a VM, make sure you have `git` installed and the `coder` user has access to the following files

```sh
# /home/coder/.gitconfig
[credential]
  helper = store
```

```sh
# /home/coder/.gitconfig

# GitHub example:
https://your-github-username:your-github-pat@github.com
```

If you are running Coder on Docker or Kubernetes, `git` is pre-installed in the Coder image. However, you still need to mount credentials. This can be done via a Docker volume mount or Kubernetes secrets.

### Passing git credentials in Kubernetes

First, create a `.gitconfig` and `.git-credentials` file on your local machine. You may want to do this in a temporary directory to avoid conflicting with your own git credentials.

Next, create the secret in Kubernetes. Be sure to do this in the same namespace that Coder is installed in.

```sh
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
