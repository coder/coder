# Add a programming language to your template

Now that you've finished [Launch your first workspace](./launch-workspace.md),
you can add another language toolchain to every workspace you create.

The Quickstart template installs a language only when the workspace owner selects it from the **Programming Languages** parameter.
In this guide, you add Ruby as an option,
watch a deliberate mistake fail,
and fix it so selecting Ruby installs a working Ruby toolchain.

> [!NOTE]
> This guide assumes your Quickstart template is open for editing.
> If it's not, refer to [Customize workspace startup](./customize-workspace-startup.md#open-the-template-for-editing).

## What you'll do

- ✅ Add a Ruby option to the **Programming Languages** parameter.
- ✅ Learn why the option alone doesn't install Ruby.
- ✅ Install the Ruby toolchain when a workspace starts.

## Parameters in brief

A parameter is a question Coder asks when someone creates a workspace.
Each parameter comes from a `coder_parameter` [data source](https://developer.hashicorp.com/terraform/language/data-sources) in the template.
A data source reads input;
it doesn't build infrastructure on its own.

The **Programming Languages** parameter is a multi-select list,
and each language the reader can choose is an `option` block:

```tf
data "coder_parameter" "languages" {
  name      = "languages"
  type      = "list(string)"
  form_type = "multi-select"

  option {
    name  = "Python"
    value = "python"
    icon  = "/icon/python.svg"
  }
  # ...more options
}
```

The parameter controls the form.
It doesn't install anything.
A separate startup script reads the selected values and installs each toolchain.
That split matters,
because the rest of this guide builds on it.

## Step 1: Add the Ruby option

Open `main.tf` and find the `data "coder_parameter" "languages"` block.
Add a Ruby `option` alongside the existing options:

```tf
  option {
    name  = "Ruby"
    value = "ruby"
    icon  = "/icon/ruby.svg"
  }
```

> [!IMPORTANT]
> The `option` block must sit at the same indentation as the other `option` blocks inside the parameter.
> Coder reads the parameter's choices from these blocks,
> so a misplaced `option` doesn't appear in the form.

Push a new version of the template:

```sh
coder templates push -d ~/coder-quickstart -y quickstart
```

Create a workspace from the template.
Ruby now appears in the **Programming Languages** list.
Select it,
create the workspace,
and open a terminal once the workspace starts.

## Step 2: Watch the option fail on its own

Check the Ruby version in the workspace terminal:

```sh
ruby --version
```

The command fails:

```text
ruby: command not found
```

The option appeared in the form,
you selected it,
and Ruby is still missing.
Nothing installed Ruby.
The parameter changed the question Coder asked,
but no code acted on the answer.
This is the split from [Parameters in brief](#parameters-in-brief):
the parameter is the form,
and a startup script is what builds the workspace.

## Step 3: Install Ruby when the workspace starts

The template installs each selected language from `install-languages.sh.tftpl`,
a startup script that runs when the workspace boots.
Open that file and add a branch that installs Ruby when the reader selects it:

```sh
if echo "$LANGUAGES" | grep -q "ruby"; then
  if command -v ruby >/dev/null 2>&1; then
    echo "Ruby: $(ruby --version | head -1)"
  else
    echo "Installing Ruby toolchain..."
    apt_update
    sudo apt-get install -y -qq ruby-full
    echo "Installed Ruby: $(ruby --version | head -1)"
  fi
fi
```

The script installs Ruby with `apt-get`,
the package manager built into the workspace image.

> [!WARNING]
> Use the package manager the workspace image provides,
> not a personal one.
> If you replace the `apt-get` line with `brew install ruby`,
> the build fails:
> the `codercom/enterprise-base:ubuntu` image doesn't include Homebrew,
> so the workspace logs `brew: command not found` and Ruby never installs.
> The image ships `apt-get`,
> so install system packages with it,
> as the rest of this script does.
> Use `apt-get install -y ruby-full` instead.
> To install a personal tool like a Homebrew formula in your own workspace,
> refer to [Install your own command-line tools](./install-command-line-tools.md).

Push the template again:

```sh
coder templates push -d ~/coder-quickstart -y quickstart
```

Create a fresh workspace with Ruby selected,
open a terminal,
and check the version again:

```sh
ruby --version
```

This time the workspace reports a Ruby version.

## What just happened

You changed two different things to add one language:

- The `coder_parameter` `option` block added Ruby to the workspace creation form.
- The startup script installed the Ruby toolchain when a workspace owner selected Ruby.

A parameter collects a choice.
A startup script acts on it.
A new language needs both.

## What's next?

Now that you added a language, [install your own command-line tools](./install-command-line-tools.md).

## Learn more

- [Parameters](../../admin/templates/extending-templates/parameters.md) in the Coder documentation
- [Terraform data sources](https://developer.hashicorp.com/terraform/language/data-sources)
- [Terraform types](https://developer.hashicorp.com/terraform/language/expressions/types) for parameter values
