# Replace template code with a registry module

Now that you've built a workspace from the Quickstart template in [Launch your first workspace](../index.md), you can replace hand-written template code with a reusable Coder Registry module.
The [`coder/jetbrains`](https://registry.coder.com/modules/coder/jetbrains) module configures JetBrains IDEs in a few lines, and any template can reuse it.

In this guide, you replace the template's hand-written JetBrains code with the `coder/jetbrains` module, publish the change as a new template version, and roll back to the previous version.
Because a template version is a snapshot, you can make a large change like this one and reverse it with a single command.

> [!NOTE]
> This guide assumes your Quickstart template is open for editing.
> If it isn't, refer to [Open the template for editing](./index.md#open-the-template-for-editing).

## What you'll do

- ✅ Replace the `jetbrains_ides` parameter and the `jetbrains_selected` local with the `coder/jetbrains` module.
- ✅ Keep the language-to-IDE mapping, so selecting Rust still gives RustRover.
- ✅ Publish a new template version, then roll back to the previous one.

## What the module replaces

Your template sets up JetBrains IDEs in three parts:

- A `data "coder_parameter" "jetbrains_ides"` block with an `option` for each IDE.
- A `jetbrains_by_language` local that maps each language to a JetBrains IDE, so Rust maps to RustRover and Go maps to GoLand.
- A `jetbrains_selected` local that reads the parameter.

The parameter block alone is about 60 lines.
A [module](https://developer.hashicorp.com/terraform/language/modules) is a reusable bundle of Terraform that you pull in by reference.
The `coder/jetbrains` module replaces the parameter and the `jetbrains_selected` local, and it reuses the `jetbrains_by_language` mapping, so the language-to-IDE behavior doesn't change.
Because the module lives in the [Coder Registry](https://registry.coder.com), you get a tested implementation that resolves each IDE's build version for you.

## Step 1: Replace the parameter with the module

Open `main.tf` and make three changes.

First, delete the `data "coder_parameter" "jetbrains_ides"` block.
This is the roughly 60-line block that starts with:

```tf
data "coder_parameter" "jetbrains_ides" {
  count = contains(local.ides, "jetbrains") ? 1 : 0
  # ...
}
```

Second, delete the `jetbrains_selected` local, which read that parameter:

```tf
jetbrains_selected = contains(local.ides, "jetbrains") ? jsondecode(data.coder_parameter.jetbrains_ides[0].value) : []
```

Keep the `jetbrains_by_language` map and the `jetbrains_ides_from_languages` local.
The module uses them to turn the selected languages into IDE codes.

Third, add the `coder/jetbrains` module to the IDE modules section, next to the other IDE modules:

```tf
module "jetbrains" {
  count    = data.coder_workspace.me.start_count * (contains(local.ides, "jetbrains") && length(local.jetbrains_ides_from_languages) > 0 ? 1 : 0)
  source   = "registry.coder.com/coder/jetbrains/coder"
  version  = "~> 1.0"
  agent_id = coder_agent.main.id
  folder   = "/home/coder"
  default  = toset(local.jetbrains_ides_from_languages)
}
```

The `default` argument does the work.
When you pass IDE codes to `default`, the module creates a button for each of those IDEs instead of showing a separate picker.
Because `jetbrains_ides_from_languages` maps the selected languages to their IDEs, a workspace that selects Rust and JetBrains gets RustRover.

One reference to the old parameter remains.
The Backend (Go) preset sets `jetbrains_ides`, so remove that line:

```tf
data "coder_workspace_preset" "backend_go" {
  name = "Backend (Go)"
  icon = "/icon/go.svg"
  parameters = {
    languages = jsonencode(["go"])
    ides      = jsonencode(["code-server", "jetbrains"])
    git_repo  = ""
  }
}
```

The Go preset still selects JetBrains, and the language mapping gives it GoLand.

## Step 2: Publish a new version

Publish the edited template as a new version.

<div class="tabs">

### UI

In the web editor, make the changes to `main.tf`.
Select **Build**, wait for the build to pass, then select **Publish**.

### CLI

Edit `~/coder-quickstart/main.tf`, then publish a new version:

```sh
coder templates push -d ~/coder-quickstart -y quickstart
```

</div>

Coder validates the Terraform and creates a new active version.
New workspaces use it right away, and existing workspaces adopt it on their next build.

## Step 3: Confirm the module works

Create a workspace from the template.
Select Rust as the language and JetBrains IDEs as the editor.
When the workspace starts, the dashboard shows a RustRover button that the module created, with no hand-written parameter behind it.
To open it, use [JetBrains Toolbox](../../user-guides/workspace-access/jetbrains/toolbox.md).

## Roll back to a previous version

A template version is a snapshot, so you can try the module and return to the earlier version at any time.
List the template's versions:

```sh
coder templates versions list quickstart
```

Find the version from before your push, then promote it back to active:

```sh
coder templates versions promote --template quickstart --template-version <previous-version>
```

You can do the same in the dashboard from **Templates** > **quickstart** > **Versions** by promoting the earlier version.
New workspaces use the promoted version, and existing workspaces return to it on their next build.

## What just happened

You replaced hand-written template code with a reusable module:

- The `coder/jetbrains` module now owns the IDE buttons and resolves each IDE's build version.
- The template keeps only the language mapping and a single module block, which removes about 60 lines.
- The same module block works in any template: add it, then point `agent_id` and `folder` at that template's agent and project directory.

Reusing a tested module instead of copying a parameter block is what keeps templates portable.
Because you published the change as a version, the rollback command reverses it whenever you want.

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
  # Used as the default for the coder/jetbrains module.
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

module "jetbrains" {
  count    = data.coder_workspace.me.start_count * (contains(local.ides, "jetbrains") && length(local.jetbrains_ides_from_languages) > 0 ? 1 : 0)
  source   = "registry.coder.com/coder/jetbrains/coder"
  version  = "~> 1.0"
  agent_id = coder_agent.main.id
  folder   = "/home/coder"
  default  = toset(local.jetbrains_ides_from_languages)
}

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
    languages = jsonencode(["go"])
    ides      = jsonencode(["code-server", "jetbrains"])
    git_repo  = ""
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

You finished the Customize your template series, and your template now pulls its JetBrains IDEs from the Coder Registry.

This is the last guide in the series.
To keep going, explore more of what Coder offers:

- [Manage workspaces for your team](../../user-guides/workspace-management.md).
- [Try Coder Agents](../../ai-coder/agents/getting-started.md), the chat interface and API for delegating work to coding agents.

Or revisit the [Customize your template overview](./index.md) for the full list of guides.

## Learn more

- [Add modules to a template](../../admin/templates/extending-templates/modules.md)
- [`coder/jetbrains` module](https://registry.coder.com/modules/coder/jetbrains)
- [Template change management](../../admin/templates/managing-templates/change-management.md)
- [JetBrains Toolbox](../../user-guides/workspace-access/jetbrains/toolbox.md)
