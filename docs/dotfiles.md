# Dotfiles

<!-- markdown-link-check-disable -->

Coder offers the `coder dotfiles <repo>` command which simplifies workspace
personalization. Our behavior is consistent with Codespaces, so
[their documentation](https://docs.github.com/en/codespaces/customizing-your-codespace/personalizing-codespaces-for-your-account#dotfiles)
explains how it loads your repo.

<!-- markdown-link-check-enable -->

You can read more on dotfiles best practices [here](https://dotfiles.github.io).

## Templates

Templates can prompt users for their dotfiles repo using the following pattern:

```hcl
variable "dotfiles_uri" {
  description = <<-EOF
  Dotfiles repo URI (optional)

  see https://dotfiles.github.io
  EOF
    # The codercom/enterprise-* images are only built for amd64
  default = ""
}

resource "coder_agent" "main" {
  ...
  startup_script = var.dotfiles_uri != "" ? "coder dotfiles -y ${var.dotfiles_uri}" : null
}
```

[Here's a complete example.](https://github.com/coder/coder/tree/main/examples/templates/docker-with-dotfiles#how-it-works)

## Persistent Home

Sometimes you want to support personalization without
requiring dotfiles.

In such cases:

- Mount a persistent volume to the `/home` directory
- Set the `startup_script` to call a `~/personalize` script that the user can edit

```hcl
resource "coder_agent" "main" {
  ...
  startup_script = "/home/coder/personalize"
}
```

The user can even fill `personalize` with `coder dotfiles <repo>`, but those
looking for a simpler approach can inline commands like so:

```bash
#!/bin/bash
sudo apt update
# Install some of my favorite tools every time my workspace boots
sudo apt install -y neovim fish cargo
```

## Install or Setup script support

User can have either of the following setup script files in their dotfiles repo:

- install.sh
- install
- bootstrap.sh
- bootstrap
- script/bootstrap
- setup.sh
- setup
- script/setup

If any of the above files are found in the specified order, Coder will execute the first match, and the rest will be skipped.

The Setup scripts must be executable. If they are not, Coder will not execute them. If the script is not executable, you can make it executable by running:

```bash
cd <path_to_dotfiles_repo>

chmod +x <script_name>

git commit -m "Make <script_name> executable" <script_name>

git push
```
