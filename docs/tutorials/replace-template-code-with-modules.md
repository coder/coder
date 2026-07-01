# Replace template code with a registry module

The Coder Quickstart template configures JetBrains IDEs with a hand-written parameter and a language-to-IDE mapping.
The [`coder/jetbrains` module](https://registry.coder.com/modules/coder/jetbrains) from the Coder Registry does the same work in a few lines, and any template can reuse it.

In this tutorial, you replace the hand-written JetBrains block with the `coder/jetbrains` module, push the change as a new template version, and roll back to the previous version.

## Before you start

You need the following:

- A `quickstart` template in your deployment, created from the [Quickstart](../get-started/index.md).
- The `coder` CLI, authenticated with `coder login`, and permission to edit templates.
- [JetBrains Toolbox](https://www.jetbrains.com/toolbox-app/) 2.7 or later where you open the IDEs, plus Coder 2.24 or later for the module.

## What the module replaces

Your template's `main.tf` sets up JetBrains IDEs in three parts:

- A `data "coder_parameter" "jetbrains_ides"` block with an `option` for each IDE.
- A `jetbrains_by_language` local that maps each language to a JetBrains IDE, so Rust maps to RustRover and Go maps to GoLand.
- A `jetbrains_selected` local that reads the parameter.

The parameter block alone is about 60 lines.
The `coder/jetbrains` module replaces the parameter and the `jetbrains_selected` local.
It reuses the `jetbrains_by_language` mapping, so the language-to-IDE behavior does not change.

## 1. Pull the template source

Download the active version of the template to a local directory, then change into it:

```sh
coder templates pull quickstart ./quickstart
cd quickstart
```

## 2. Replace the parameter with the module

Open `main.tf`.
Delete the `data "coder_parameter" "jetbrains_ides"` block and the `jetbrains_selected` local.
Keep the `jetbrains_by_language` and `jetbrains_ides_from_languages` locals.

The template includes a commented-out `coder/jetbrains` module.
Replace it with this working block:

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

## 3. Push the change as a new version

Push the directory as a new version of the template:

```sh
coder templates push quickstart -d . -y
```

Coder validates the Terraform and creates a new active version.
New workspaces use it right away, and existing workspaces adopt it on their next build.

## 4. Confirm the module works

Create a workspace from the template.
Select Rust as the language and JetBrains IDEs as the editor.
When the workspace starts, the dashboard shows a RustRover button that the module created, with no hand-written parameter behind it.

## Roll back to the previous version

A template change is reversible, so you can try the module and return to the earlier version at any time.
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

## How the module keeps the template lightweight

The hand-written setup needed a parameter with an `option` for every IDE, plus a local to track the selection.
The module owns the IDE buttons and resolves each IDE's build version, so the template keeps only the language mapping and a single module block.
The change removes about 60 lines.

The same block works in any template.
To offer JetBrains IDEs elsewhere, add the module and point `agent_id` and `folder` at that template's agent and project directory.
Reusing a tested module instead of copying parameter blocks is what keeps templates portable.

## Learn more

- [Add modules to a template](../admin/templates/extending-templates/modules.md)
- [`coder/jetbrains` module](https://registry.coder.com/modules/coder/jetbrains)
- [Template change management](../admin/templates/managing-templates/change-management.md)
- [JetBrains Toolbox](../user-guides/workspace-access/jetbrains/toolbox.md)
