# Install your own command-line tools

Now that you [launched your first workspace](../index.md), you can add your favorite command-line tools to every workspace.

The Quickstart template installs system languages through the **Programming Languages** parameter, but it doesn't carry the small command-line tools you may often use, such as [`bat`](https://github.com/sharkdp/bat) or [`ripgrep`](https://github.com/BurntSushi/ripgrep).
You can install those yourself with a package manager like [Homebrew](https://brew.sh/) or [mise](https://mise.jdx.dev/).

In this guide, you install both Homebrew and mise, install a tool with each, and learn which installs survive a workspace restart and why.
You then change the template so the Homebrew tools persist too, and finish by making your tools install in every new workspace automatically.

> [!NOTE]
> This guide works inside a running workspace from the Quickstart template.
> Most of it runs in the workspace, but the last two steps edit the template so Homebrew persists and every new workspace ships with your tools.

## What you'll do

- ✅ Install command-line tools with [Homebrew](https://brew.sh/) and [mise](https://mise.jdx.dev/) into your workspace.
- ✅ Restart the workspace and see which tools persist.
- ✅ Learn why one persists and the other doesn't.
- ✅ Wire up Homebrew so its tools persist too.
- ✅ Preinstall your tools in every new workspace from the template.

## What persists in a workspace

A Quickstart workspace keeps your home directory, `/home/coder`, on a persistent volume.
Everything outside `/home/coder` comes from the workspace image, and Coder rebuilds it from that image every time the workspace starts.

A tool survives a restart only when both of these are true:

- The tool installs into `/home/coder`.
- Your shell finds the tool through a file in `/home/coder`, such as `.bashrc`.

You'll install tools two ways and restart to see this rule decide which ones stay, change the template so Homebrew follows the rule too, then preinstall your tools in every new workspace.

## Step 1: Install Homebrew and mise

Open a terminal in your workspace.

Install [Homebrew](https://brew.sh/) with its setup script:

```sh
NONINTERACTIVE=1 /bin/bash -c "$(curl -fsSL https://raw.githubusercontent.com/Homebrew/install/HEAD/install.sh)"
```

Homebrew installs to `/home/linuxbrew/.linuxbrew`.
Add it to your shell so the `brew` command is available:

```sh
echo 'eval "$(/home/linuxbrew/.linuxbrew/bin/brew shellenv)"' >> ~/.bashrc
eval "$(/home/linuxbrew/.linuxbrew/bin/brew shellenv)"
```

Confirm Homebrew is available before you continue:

```sh
brew --version
```

Install [mise](https://mise.jdx.dev/) with its setup script:

```sh
curl -fsSL https://mise.run | sh
```

mise installs to `~/.local/bin/mise`, inside your home directory.
Activate it so every new shell loads it:

```sh
echo 'eval "$(~/.local/bin/mise activate bash)"' >> ~/.bashrc
```

Activation only takes effect in shells that start after this change, so open a new terminal (or run `source ~/.bashrc`) before you use mise.
Until you do, `mise doctor` reports that mise isn't activated, which is expected at this point.
For other shells, refer to [Activate mise](https://mise.jdx.dev/getting-started.html#activate-mise).

Open a new terminal so both the Homebrew and mise changes take effect, then confirm each manager runs:

```sh
brew --version
mise --version
```

> [!NOTE]
> If you manage `~/.bashrc` with [dotfiles](../../user-guides/workspace-dotfiles.md), add the `brew shellenv` and `mise activate` lines to the `.bashrc` in your dotfiles repository instead, so applying your dotfiles doesn't overwrite them.

## Step 2: Install a tool with each manager

Install [`ripgrep`](https://github.com/BurntSushi/ripgrep) with Homebrew:

```sh
brew install ripgrep
```

Install [`bat`](https://github.com/sharkdp/bat) with mise:

```sh
mise use -g bat
```

Confirm both tools run:

```sh
rg --version
bat --version
```

Both work.
So far, the two package managers look interchangeable.

## Step 3: Restart the workspace and compare

Restart the workspace.
The restart rebuilds the container from the image and keeps only your home directory.

<div class="tabs">

### UI

Open your workspace in the Coder dashboard and select **Restart**.
When it's back, reconnect by reopening the web terminal.

### CLI

From a terminal on your own machine, restart the workspace by name, then reconnect when it's back:

```sh
coder restart <your-workspace>
coder ssh <your-workspace>
```

</div>

When you reconnect, your shell prints an error before you run anything:

```txt
bash: /home/linuxbrew/.linuxbrew/bin/brew: No such file or directory
```

That's the first sign something changed.
Your `.bashrc` still tries to load Homebrew, but the restart removed it.
Check each tool to see what survived.

`bat`, installed with mise, still works:

```sh
bat --version
```

```txt
bat 0.26.1
```

`rg`, installed with Homebrew, is gone:

```sh
rg --version
```

```txt
bash: rg: command not found
```

So is `brew` itself:

```sh
brew --version
```

```txt
bash: brew: command not found
```

mise installed `bat` under `/home/coder`, which persists, so `bat` survived.
Homebrew installed `ripgrep` to `/home/linuxbrew`, outside `/home/coder`, so the rebuild discarded Homebrew and every formula you installed with it.
The `brew shellenv` line stayed in your `.bashrc` because it lives in `/home/coder`, which is why your shell still tries to load the missing `brew` and prints the error above.

## Step 4: Make Homebrew survive restarts

To make Homebrew survive a restart, you'll edit the template and add a persistent volume.
The volume backs `/home/linuxbrew`, the prefix where Homebrew installs, so Homebrew and its formulae stay between restarts.

> [!NOTE]
> This step assumes your Quickstart template is open for editing.
> If it's not, you can edit the template from the web by finding the template, selecting the three dots menu, and selecting **Edit files**.
> Refer to [Customize workspace startup](./index.md#open-the-template-for-editing) for more information.

In `main.tf`, add a volume for Homebrew's directory next to the existing `home_volume`:

```tf
resource "docker_volume" "homebrew_volume" {
  name = "coder-${data.coder_workspace.me.id}-homebrew"
  lifecycle {
    ignore_changes = all
  }
}
```

Then mount it in the `docker_container "workspace"` resource, alongside the block that mounts `/home/coder`:

```tf
  volumes {
    container_path = "/home/linuxbrew"
    volume_name    = docker_volume.homebrew_volume.name
    read_only      = false
  }
```

Publish the change and restart the workspace:

<div class="tabs">

### UI

In the web editor, make the edits above in `main.tf`.
Select **Build**, wait for the build to pass, then select **Publish**.
On your workspace's home tab, select **Update and restart**.

### CLI

Make the edits in `~/coder-quickstart/main.tf`, then publish and update by name:

```sh
coder templates push -d ~/coder-quickstart -y quickstart
coder update <your-workspace>
```

</div>

The restart gives you a persistent but empty `/home/linuxbrew`.
Your earlier Homebrew install is gone, so install it once more.
This time it lands on the volume:

```sh
NONINTERACTIVE=1 /bin/bash -c "$(curl -fsSL https://raw.githubusercontent.com/Homebrew/install/HEAD/install.sh)"
```

The `brew shellenv` line is still in your `.bashrc` from Step 1, so open a new terminal to load it, then reinstall `ripgrep`:

```sh
brew install ripgrep
```

Restart the workspace once more and reconnect.
This time the startup error is gone, and both tools report their versions:

```sh
rg --version
brew --version
```

Homebrew now persists because `/home/linuxbrew` lives on its own volume, the same way mise's tools persist because they live in `/home/coder`.

## Step 5: Install your tools in every new workspace

Steps 1 through 4 set up your tools in this workspace.
To give everyone who uses the template the same tools, install them from the template so every new workspace starts with them.

The template already installs languages on every start with the `install_languages` script.
You'll add a second script that installs your command-line tools the same way.

> [!NOTE]
> This step edits the template.
> If it isn't open for editing, refer to [Customize workspace startup](./index.md#open-the-template-for-editing).

mise is the lighter choice here.
It installs prebuilt binaries into `/home/coder`, which already persists, so the first start stays quick and later starts reuse the tools.

In `main.tf`, add a script next to the existing `install_languages` resource:

```tf
resource "coder_script" "install_tools" {
  agent_id     = coder_agent.main.id
  display_name = "Install Tools"
  icon         = "/icon/terminal.svg"
  run_on_start = true
  script       = <<-EOT
    #!/usr/bin/env bash
    set -e

    # Install mise on first start. It lives in /home/coder, so later starts reuse it.
    if [ ! -x "$HOME/.local/bin/mise" ]; then
      curl -fsSL https://mise.run | sh
    fi
    export PATH="$HOME/.local/bin:$HOME/.local/share/mise/shims:$PATH"

    # Install the tools for everyone who uses the template, tracking the latest release.
    mise use -g ripgrep@latest bat@latest

    # Load mise in new interactive shells so the tools are on PATH.
    if ! grep -qs 'mise activate' "$HOME/.bashrc"; then
      echo 'eval "$(mise activate bash)"' >> "$HOME/.bashrc"
    fi
  EOT
}
```

`mise use -g ripgrep@latest bat@latest` writes the tools to mise's global config at `~/.config/mise/config.toml` and installs them, so every workspace from the template resolves the same versions.

Publish the change and apply it to your workspace:

<div class="tabs">

### UI

In the web editor, add the `install_tools` resource to `main.tf`.
Select **Build**, wait for the build to pass, then select **Publish**.
On your workspace's home tab, select **Update and restart**.

### CLI

Add the resource in `~/coder-quickstart/main.tf`, then publish and update by name:

```sh
coder templates push -d ~/coder-quickstart -y quickstart
coder update <your-workspace>
```

</div>

When the workspace is back, confirm both tools run without installing anything by hand:

```sh
rg --version
bat --version
```

The script runs on every start, so every new workspace from the template now ships with `ripgrep` and `bat`.

<details>

<summary>Use Homebrew and a Brewfile instead</summary>

If you'd rather manage these tools with Homebrew, install them from a [Brewfile](https://docs.brew.sh/Brew-Bundle-and-Brewfile) on every start.
This approach relies on the `/home/linuxbrew` volume you added in Step 4, so Homebrew and its formulae persist between restarts.

Use this `install_tools` script in place of the mise version:

```tf
resource "coder_script" "install_tools" {
  agent_id     = coder_agent.main.id
  display_name = "Install Tools"
  icon         = "/icon/terminal.svg"
  run_on_start = true
  script       = <<-EOT
    #!/usr/bin/env bash
    set -e

    # Install Homebrew on first start, while the volume is still empty.
    if [ ! -x /home/linuxbrew/.linuxbrew/bin/brew ]; then
      NONINTERACTIVE=1 /bin/bash -c "$(curl -fsSL https://raw.githubusercontent.com/Homebrew/install/HEAD/install.sh)"
    fi
    eval "$(/home/linuxbrew/.linuxbrew/bin/brew shellenv)"

    # Write a Brewfile and install everything it lists.
    printf 'brew "ripgrep"\nbrew "bat"\n' > "$HOME/Brewfile"
    brew bundle install --file="$HOME/Brewfile"

    # Load Homebrew in new shells so the tools are on PATH.
    if ! grep -qs 'brew shellenv' "$HOME/.bashrc"; then
      echo 'eval "$(/home/linuxbrew/.linuxbrew/bin/brew shellenv)"' >> "$HOME/.bashrc"
    fi
  EOT
}
```

Homebrew installs more slowly than mise on a fresh volume because it downloads larger packages, which is why mise is the default here.

</details>

## What just happened

The two package managers behaved differently for one reason: where each one installs.

- mise installs into `~/.local/share/mise`, inside your home directory, and activates from `~/.bashrc`.
  Both are in `/home/coder`, so its tools persist with no template change.
- Homebrew installs to `/home/linuxbrew`, outside `/home/coder`, so its tools are discarded on every restart until you mount that path on a persistent volume.

To keep a tool, choose the approach that matches who needs it:

- For a tool that's yours alone, install it with mise.
  It persists through restarts with no template change.
- To keep your Homebrew tools, mount `/home/linuxbrew` on a persistent volume, as you did in Step 4.
  This is a template change, so it affects everyone who uses the template.
- To preinstall a tool in every new workspace, add it to the template's startup script, as you did in Step 5 with mise.
  You can also install system packages with `apt-get`, as in [Add a programming language](./add-a-language.md), or bake the tool into the workspace image.

The rule underneath all of these: a tool persists when it lives in a part of the workspace that persists.
Refer to [Resource persistence](../../admin/templates/extending-templates/resource-persistence.md) for how Coder decides what survives a restart.

## Final code

<details>

<summary>The complete <code>main.tf</code></summary>

Your `main.tf` after this guide's changes, starting from the Quickstart template:

```tf
terraform {
  required_providers {
    coder = {
      source = "coder/coder"
    }
    docker = {
      source = "kreuzwerker/docker"
    }
    external = {
      source = "hashicorp/external"
    }
  }
}

variable "docker_socket" {
  default     = ""
  description = "(Optional) Docker socket URI"
  type        = string
}

provider "docker" {
  host = var.docker_socket != "" ? var.docker_socket : null
}

data "coder_provisioner" "me" {}
data "coder_workspace" "me" {}
data "coder_workspace_owner" "me" {}

# --- Parameters ---

data "coder_parameter" "languages" {
  name         = "languages"
  display_name = "Programming Languages"
  description  = "Select the languages to pre-install in your workspace"
  type         = "list(string)"
  form_type    = "multi-select"
  default      = jsonencode(["python"])
  mutable      = true
  icon         = "/icon/code.svg"
  order        = 1

  option {
    name  = "Python"
    value = "python"
    icon  = "/icon/python.svg"
  }
  option {
    name  = "Node.js"
    value = "nodejs"
    icon  = "/icon/nodejs.svg"
  }
  option {
    name  = "Go"
    value = "go"
    icon  = "/icon/go.svg"
  }
  option {
    name  = "Rust"
    value = "rust"
    icon  = "/icon/rust.svg"
  }
  option {
    name  = "Java"
    value = "java"
    icon  = "/icon/java.svg"
  }
  option {
    name  = "C/C++"
    value = "cpp"
    icon  = "/icon/cpp.svg"
  }
}

data "coder_parameter" "ides" {
  name         = "ides"
  display_name = "IDEs & Editors"
  description  = "Select the development environments for your workspace"
  type         = "list(string)"
  form_type    = "multi-select"
  default      = jsonencode(["code-server"])
  mutable      = true
  icon         = "/icon/code.svg"
  order        = 2

  option {
    name  = "VS Code (Browser)"
    value = "code-server"
    icon  = "/icon/code.svg"
  }
  option {
    name  = "Cursor"
    value = "cursor"
    icon  = "/icon/cursor.svg"
  }
  option {
    name  = "JetBrains IDEs"
    value = "jetbrains"
    icon  = "/icon/jetbrains.svg"
  }
  option {
    name  = "Zed"
    value = "zed"
    icon  = "/icon/zed.svg"
  }
  option {
    name  = "Windsurf"
    value = "windsurf"
    icon  = "/icon/windsurf.svg"
  }
}

# Shown only when "JetBrains IDEs" is selected in the IDEs parameter.
# Pre-selects IDEs that match the chosen languages.
data "coder_parameter" "jetbrains_ides" {
  count        = contains(local.ides, "jetbrains") ? 1 : 0
  name         = "jetbrains_ides"
  display_name = "JetBrains IDEs"
  description  = "Select the JetBrains IDEs to install"
  type         = "list(string)"
  form_type    = "multi-select"
  default      = jsonencode(local.jetbrains_ides_from_languages)
  mutable      = true
  icon         = "/icon/jetbrains.svg"
  order        = 3

  option {
    name  = "IntelliJ IDEA"
    value = "IU"
    icon  = "/icon/intellij.svg"
  }
  option {
    name  = "PyCharm"
    value = "PY"
    icon  = "/icon/pycharm.svg"
  }
  option {
    name  = "GoLand"
    value = "GO"
    icon  = "/icon/goland.svg"
  }
  option {
    name  = "WebStorm"
    value = "WS"
    icon  = "/icon/webstorm.svg"
  }
  option {
    name  = "RustRover"
    value = "RR"
    icon  = "/icon/rustrover.svg"
  }
  option {
    name  = "CLion"
    value = "CL"
    icon  = "/icon/clion.svg"
  }
  option {
    name  = "PhpStorm"
    value = "PS"
    icon  = "/icon/phpstorm.svg"
  }
  option {
    name  = "RubyMine"
    value = "RM"
    icon  = "/icon/rubymine.svg"
  }
  option {
    name  = "Rider"
    value = "RD"
    icon  = "/icon/rider.svg"
  }
}

data "coder_parameter" "git_repo" {
  name         = "git_repo"
  display_name = "Git Repository (Optional)"
  description  = "URL of a Git repository to clone into your workspace (leave empty to skip)"
  type         = "string"
  default      = ""
  mutable      = true
  icon         = "/icon/git.svg"
  order        = 4
}

# --- Locals ---

locals {
  username  = data.coder_workspace_owner.me.name
  languages = jsondecode(data.coder_parameter.languages.value)
  ides      = jsondecode(data.coder_parameter.ides.value)

  # Map selected languages to the relevant JetBrains IDE product codes.
  # Used as the default for the JetBrains IDE selector parameter.
  jetbrains_by_language = {
    python = ["PY"]
    go     = ["GO"]
    java   = ["IU"]
    nodejs = ["WS"]
    rust   = ["RR"]
    cpp    = ["CL"]
  }
  jetbrains_ides_from_languages = distinct(flatten([
    for lang in local.languages : lookup(local.jetbrains_by_language, lang, [])
  ]))

  # The actual JetBrains IDEs to install, from the user's selection
  # in the conditional JetBrains parameter (or empty if not shown).
  jetbrains_selected = contains(local.ides, "jetbrains") ? jsondecode(data.coder_parameter.jetbrains_ides[0].value) : []
}

# --- Agent ---

resource "coder_agent" "main" {
  arch           = data.coder_provisioner.me.arch
  os             = "linux"
  startup_script = <<-EOT
    set -e
    if [ ! -f ~/.init_done ]; then
      cp -rT /etc/skel ~
      touch ~/.init_done
    fi
  EOT

  env = {
    GIT_AUTHOR_NAME     = coalesce(data.coder_workspace_owner.me.full_name, data.coder_workspace_owner.me.name)
    GIT_AUTHOR_EMAIL    = "${data.coder_workspace_owner.me.email}"
    GIT_COMMITTER_NAME  = coalesce(data.coder_workspace_owner.me.full_name, data.coder_workspace_owner.me.name)
    GIT_COMMITTER_EMAIL = "${data.coder_workspace_owner.me.email}"
  }

  metadata {
    display_name = "CPU Usage"
    key          = "0_cpu_usage"
    script       = "coder stat cpu"
    interval     = 10
    timeout      = 1
  }

  metadata {
    display_name = "RAM Usage"
    key          = "1_ram_usage"
    script       = "coder stat mem"
    interval     = 10
    timeout      = 1
  }

  metadata {
    display_name = "Home Disk"
    key          = "3_home_disk"
    script       = "coder stat disk --path $${HOME}"
    interval     = 60
    timeout      = 1
  }
}

# --- Language installation ---
# All languages install in a single script to avoid apt-get lock
# conflicts (coder_script resources run in parallel).

resource "coder_script" "install_languages" {
  count              = length(local.languages) > 0 ? 1 : 0
  agent_id           = coder_agent.main.id
  display_name       = "Install Languages"
  icon               = "/icon/code.svg"
  run_on_start       = true
  start_blocks_login = true
  script = templatefile("${path.module}/install-languages.sh.tftpl", {
    LANGUAGES = join(",", local.languages)
  })
}

# --- Tool installation ---

resource "coder_script" "install_tools" {
  agent_id     = coder_agent.main.id
  display_name = "Install Tools"
  icon         = "/icon/terminal.svg"
  run_on_start = true
  script       = <<-EOT
    #!/usr/bin/env bash
    set -e

    # Install mise on first start. It lives in /home/coder, so later starts reuse it.
    if [ ! -x "$HOME/.local/bin/mise" ]; then
      curl -fsSL https://mise.run | sh
    fi
    export PATH="$HOME/.local/bin:$HOME/.local/share/mise/shims:$PATH"

    # Install the tools for everyone who uses the template, tracking the latest release.
    mise use -g ripgrep@latest bat@latest

    # Load mise in new interactive shells so the tools are on PATH.
    if ! grep -qs 'mise activate' "$HOME/.bashrc"; then
      echo 'eval "$(mise activate bash)"' >> "$HOME/.bashrc"
    fi
  EOT
}

# --- IDE modules ---

module "code-server" {
  count    = data.coder_workspace.me.start_count * (contains(local.ides, "code-server") ? 1 : 0)
  source   = "registry.coder.com/coder/code-server/coder"
  version  = "~> 1.0"
  agent_id = coder_agent.main.id
  order    = 1
}

module "cursor" {
  count    = data.coder_workspace.me.start_count * (contains(local.ides, "cursor") ? 1 : 0)
  source   = "registry.coder.com/coder/cursor/coder"
  version  = "~> 1.0"
  agent_id = coder_agent.main.id
  folder   = "/home/coder"
  order    = 3
}

# TODO: Re-add the coder/jetbrains module once Coder's dynamic
# parameter system respects module count for parameter visibility.
# The module's internal coder_parameter appears even when count = 0,
# creating a ghost parameter in the workspace creation form.
# module "jetbrains" {
#   count    = data.coder_workspace.me.start_count * (contains(local.ides, "jetbrains") && length(local.jetbrains_selected) > 0 ? 1 : 0)
#   source   = "registry.coder.com/coder/jetbrains/coder"
#   version  = "~> 1.0"
#   agent_id = coder_agent.main.id
#   folder   = "/home/coder"
#   default  = toset(local.jetbrains_selected)
# }

module "zed" {
  count    = data.coder_workspace.me.start_count * (contains(local.ides, "zed") ? 1 : 0)
  source   = "registry.coder.com/coder/zed/coder"
  version  = "~> 1.0"
  agent_id = coder_agent.main.id
  folder   = "/home/coder"
  order    = 5
}

module "windsurf" {
  count    = data.coder_workspace.me.start_count * (contains(local.ides, "windsurf") ? 1 : 0)
  source   = "registry.coder.com/coder/windsurf/coder"
  version  = "~> 1.0"
  agent_id = coder_agent.main.id
  folder   = "/home/coder"
  order    = 6
}

# --- Git clone ---

module "git-clone" {
  count    = data.coder_workspace.me.start_count * (data.coder_parameter.git_repo.value != "" ? 1 : 0)
  source   = "registry.coder.com/coder/git-clone/coder"
  version  = "~> 2.0"
  agent_id = coder_agent.main.id
  url      = data.coder_parameter.git_repo.value
}

# --- Presets ---

data "coder_workspace_preset" "web_dev" {
  name = "Web Development"
  icon = "/icon/nodejs.svg"
  parameters = {
    languages = jsonencode(["python", "nodejs"])
    ides      = jsonencode(["code-server"])
    git_repo  = ""
  }
}

data "coder_workspace_preset" "backend_go" {
  name = "Backend (Go)"
  icon = "/icon/go.svg"
  parameters = {
    languages      = jsonencode(["go"])
    ides           = jsonencode(["code-server", "jetbrains"])
    jetbrains_ides = jsonencode(["GO"])
    git_repo       = ""
  }
}

data "coder_workspace_preset" "data_science" {
  name = "Data Science"
  icon = "/icon/python.svg"
  parameters = {
    languages = jsonencode(["python"])
    ides      = jsonencode(["code-server"])
    git_repo  = ""
  }
}

data "coder_workspace_preset" "full_stack" {
  name = "Full Stack"
  icon = "/icon/code.svg"
  parameters = {
    languages = jsonencode(["python", "nodejs", "go"])
    ides      = jsonencode(["code-server", "cursor"])
    git_repo  = ""
  }
}

# --- Docker resources ---

resource "docker_volume" "home_volume" {
  name = "coder-${data.coder_workspace.me.id}-home"
  lifecycle {
    ignore_changes = all
  }
  labels {
    label = "coder.owner"
    value = data.coder_workspace_owner.me.name
  }
  labels {
    label = "coder.owner_id"
    value = data.coder_workspace_owner.me.id
  }
  labels {
    label = "coder.workspace_id"
    value = data.coder_workspace.me.id
  }
  labels {
    label = "coder.workspace_name_at_creation"
    value = data.coder_workspace.me.name
  }
  depends_on = []
}

resource "docker_volume" "homebrew_volume" {
  name = "coder-${data.coder_workspace.me.id}-homebrew"
  lifecycle {
    ignore_changes = all
  }
}

resource "docker_container" "workspace" {
  count    = data.coder_workspace.me.start_count
  image    = "codercom/enterprise-base:ubuntu"
  name     = "coder-${data.coder_workspace_owner.me.name}-${lower(data.coder_workspace.me.name)}"
  hostname = data.coder_workspace.me.name
  entrypoint = [
    "sh", "-c",
    replace(coder_agent.main.init_script, "/localhost|127\\.0\\.0\\.1/", "host.docker.internal"),
  ]
  env = ["CODER_AGENT_TOKEN=${coder_agent.main.token}"]
  host {
    host = "host.docker.internal"
    ip   = "host-gateway"
  }
  volumes {
    container_path = "/home/coder"
    volume_name    = docker_volume.home_volume.name
    read_only      = false
  }
  volumes {
    container_path = "/home/linuxbrew"
    volume_name    = docker_volume.homebrew_volume.name
    read_only      = false
  }
  labels {
    label = "coder.owner"
    value = data.coder_workspace_owner.me.name
  }
  labels {
    label = "coder.owner_id"
    value = data.coder_workspace_owner.me.id
  }
  labels {
    label = "coder.workspace_id"
    value = data.coder_workspace.me.id
  }
  labels {
    label = "coder.workspace_name"
    value = data.coder_workspace.me.name
  }
  depends_on = []
}
```

</details>

## What's next?

Now that you can install your own tools, [clone private repositories](./authenticate-to-github.md) so your workspaces can reach your private GitHub code.

## Learn more

- [Homebrew documentation](https://brew.sh/) for the package manager
- [mise documentation](https://mise.jdx.dev/) for the version manager
- [Resource persistence](../../admin/templates/extending-templates/resource-persistence.md) in the Coder documentation
- [Dotfiles](../../user-guides/workspace-dotfiles.md) in the Coder documentation
