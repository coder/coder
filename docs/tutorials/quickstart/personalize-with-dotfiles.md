# Personalize your workspace with dotfiles

Now that you've finished [Launch your first workspace](./launch-workspace.md),
you can make every workspace start with your own shell, editor, and Git configuration.

[Dotfiles](https://dotfiles.github.io/) are the hidden configuration files in your home directory,
such as `.bashrc`, `.gitconfig`, and `.vimrc`.
In this guide, you add the Coder dotfiles module to the template,
point a workspace at your dotfiles repository,
and apply your shell and editor settings every time a workspace starts.

> [!NOTE]
> This guide assumes your Quickstart template is open for editing.
> If it's not, refer to [Customize workspace startup](./customize-workspace-startup.md#open-the-template-for-editing).

## What you'll do

- ✅ Add the Coder dotfiles module to the template.
- ✅ Point a workspace at your dotfiles repository.
- ✅ Apply your shell and editor settings when a workspace starts.

## Modules in brief

A [module](https://developer.hashicorp.com/terraform/language/modules) is a reusable bundle of Terraform you pull in by reference instead of writing it yourself.
The [Coder Registry](https://registry.coder.com/modules) publishes modules for common workspace features,
and the [dotfiles module](https://registry.coder.com/modules/coder/dotfiles) is one of them.

A `module` block names a `source` to pull from and a `version` to pin:

```tf
# --- Dotfiles ---

module "dotfiles" {
  count    = data.coder_workspace.me.start_count
  source   = "registry.coder.com/coder/dotfiles/coder"
  version  = "1.4.2"
  agent_id = coder_agent.main.id
}
```

`count = data.coder_workspace.me.start_count` builds the module only when the workspace is running,
and `agent_id` attaches it to the workspace agent so the module runs inside the workspace.

## Step 1: Add the dotfiles module

Add the `module` block above anywhere in `main.tf`,
then push a new version of the template:

```sh
coder templates push -d ~/coder-quickstart -y quickstart
```

Create a workspace from the template.
The creation form now shows 2 new fields:

- **Dotfiles URL**, the repository to clone. The module accepts SSH (`git@host:user/repo`) and HTTPS (`https://host/user/repo`) URLs.
- **Dotfiles Branch**, the branch to apply.

Enter a dotfiles repository URL,
create the workspace,
and the module clones the repository and applies it at startup.

## Step 2: Configuration first, tools optional

Dotfiles are mainly for configuration that lives in your home directory,
the part of the workspace that [persists across rebuilds](./install-command-line-tools.md#what-persists-in-a-workspace).
Symlinking `.bashrc`, `.gitconfig`, and editor settings is exactly what the dotfiles module is for.

Dotfiles can install tools too.
A repository can run an `install.sh` script,
and many people drive tool installs from one, such as a Brewfile or a [mise](./install-command-line-tools.md) config.
That works, as long as each tool lands somewhere that persists.
The persistence rules from [Install your own command-line tools](./install-command-line-tools.md) still apply:
a tool in your home directory survives a restart,
and a tool outside it is rebuilt away unless the template keeps its location.

Whether your dotfiles carry only configuration or tools as well,
they apply every time a workspace starts,
so your settings are there from the first prompt.

## What just happened

You pulled a whole feature into the template with a few lines by referencing a module:

- The dotfiles module clones your repository and applies it inside the workspace at startup.
- Dotfiles apply your configuration at startup, and can install tools too when each tool lands somewhere that persists.

## What's next?

Now that your workspaces start with your own configuration, [authenticate to GitHub](./authenticate-to-github.md) to clone private repositories.

## Learn more

- [Dotfiles](../../user-guides/workspace-dotfiles.md) in the Coder documentation
- [Dotfiles module](https://registry.coder.com/modules/coder/dotfiles) in the Coder Registry
- [Terraform modules](https://developer.hashicorp.com/terraform/language/modules)
