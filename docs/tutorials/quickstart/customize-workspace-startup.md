# Customize workspace startup

Previously, in the [Launch your first workspace](./launch-workspace.md) guide, you downloaded the Coder CLI, started the Coder server, created your first template, and started your first workspace from that template.
If you haven't completed that guide yet, you should do so before starting this guide.

## What you'll do

The Quickstart starter template you used in the last guide features a lot of the popular programming languages and is useful for a lot of software development scenarios, but it has some drawbacks:

- It may not include all the programming languages you need to start writing code.
- It lacks any personalization through dotfiles, so users must manually configure their shell preferences and IDE settings.
- It works well for public repos, but you must manually authenticate to your source control provider with each new workspace to clone and work on private repos.

In this guide, you'll do the following:

- ✅ Learn some basic commands from the Coder CLI.
- ✅ Add a new language in the **Programming Languages** parameter.
- ✅ Add the [Dotfiles module from the Coder Registry](https://registry.coder.com/modules/coder/dotfiles) to your template.
- ✅ Add the `coder_external_auth` data source to your template.

## Templates in brief

As you learned earlier, a template is a Terraform blueprint used to define workspaces.
Templates consist of at least one or more Terraform files (`*.tf`).
You can also use Terraform templates (`*.tftpl`), a `README`, or any other file required to run and document the template.

Each Coder template needs a `required_providers` block, nested within a `terraform` block.
You can think of a provider as a plugin to integrate Terraform with a specific application, platform, or service.
Inside the `required_providers` block, you need to include the `coder` provider.
An example of this is as follows:

```hcl
terraform {
  required_providers {
    coder = {
      source = "coder/coder"
    }
  }
}
```

The `coder` provider includes different [resources](https://developer.hashicorp.com/terraform/language/resources), [data sources](https://developer.hashicorp.com/terraform/language/data-sources), and [modules](https://developer.hashicorp.com/terraform/language/modules) you can use in your templates.
A *resource* is any infrastructure object you want to manage through Terraform.
You can use *data sources* to fetch data from providers, without modifying resources.
*Modules* are collections of resources you would want to manage together and are a means to reduce defining resources multiple times.
You can add additional providers that include different Terraform objects to extend your templates.

While not required to complete this Quickstart series, you should explore the [Terraform Documentation from HashiCorp](https://developer.hashicorp.com/terraform) for information on Terraform and HCL.

## Step 1: Open the template for editing

You can modify a template in one of two ways:

- Directly in the Coder server UI
- Through the CLI

<div class="tabs">

### UI

1. Log in to Coder, and select **Templates**.
2. Find the **Coder Quickstart** template you created, and open it.
3. Select the three dots menu next to the **Create Workspace** button, and then select **Edit files**.

The template web editor will open.

### CLI

You can update templates in your favorite IDE by pulling the template locally with the Coder CLI.
With your server running in the existing terminal window, open a new terminal window and run the following:

```shell
coder login
```

`coder login` opens a new page in your web browser to generate a session token.
In this window, copy the session token, and return to the terminal.
Paste the token in the terminal.
Once you're logged in, pull the template to your local filesystem:

```shell
coder template pull quickstart ~/coder-quickstart
```

Open the `~/coder-quickstart` folder in your favorite IDE.

</div>

The template has three files:

- `install-languages.sh.tftpl`
- `main.tf`
- `README.md`

The next step focuses on the first two files. First, open `main.tf`.

## Step 2: Add support for a new language

After line 71 in `main.tf`, add the following block, matching the indentation of the text around it:

```hcl
  option {
    name  = "Ruby"
    value = "ruby"
    icon  = "/icon/ruby.svg"
  }
```

> [!NOTE]
> For most practical purposes,
> the order of HCL blocks in your `.tf` files doesn't matter.
> However, this `option` block needs to be at the same indentation level
> as the rest of the `option` blocks in the `coder_parameter` data source
> for Coder to display the value correctly.

Next, open `install-languages.sh.tftpl`, and add the following block at line 88:

```bash
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

Save the files and publish the new version of the template:

<div class="tabs">

### UI

1. Select **Build** to build the template, which compiles and verifies the template code.
2. If the build succeeds, the **Publish** button will become available. Select **Publish**.
3. Give the template a new **Version name** if you want to change the default. Keep the **Promote to active version** checkbox checked.
4. Select **Publish**.

### CLI

Save the changes to the template, and run the following command:

```shell
coder templates push -d ~/coder-quickstart -y quickstart
```

</div>

<details>
    <summary>More details for the curious</summary>

Recall from the previous guide that you can choose one or more languages to install as part of your workspace through a *parameter*.
By using parameters in your templates, you can customize the behavior of that template when a user creates a workspace from that template.
This added flexibility avoids the need to create multiple templates with only minor variations in the code and improves the experience for users.

Each parameter in the workspace creation window comes from an instance of a `coder_parameter` data source in your Terraform code.
With the `coder_parameter` data source block, you can define different types of parameters that Coder will load in the UI during workspace creation.
Parameters support different [Terraform types](https://developer.hashicorp.com/terraform/language/expressions/types), such as primitive types and lists.

The **Programming Languages** parameter is a multi-select list of strings with a predefined set of parameter options.
The definition of the **Programming Languages** parameter starts at line 31 in `main.tf`:

```hcl
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
```

A lot of unfamiliar code is here, but the following is a short summary of the important things about this block:

- The `data "coder_parameter" "languages"` block declaration means this is a `coder_parameter` data source named "languages".
  You can refer to this block as `data.coder_parameter.languages` in other parts of the Terraform code (which will be important later).
- `mutable` is set to `true`, so you can change this parameter's value.
- You have 6 options from which to choose: Python, Node.js, Go, Rust, Java, and C/C++.

Modifying the parameter block adds the option to the UI, but on its own, it won't know where to install Ruby. That's why you need to add the accompanying script to *install* Ruby when a user selects the Ruby parameter:

```bash
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

</details>

## Step 3: Add the Dotfiles module to your template

Users may want to customize their workspace startup behavior with dotfiles.
Coder offers a [Dotfiles module in the Coder Registry](https://registry.coder.com/modules/coder/dotfiles).
To add support for dotfiles in your template,
add the following block anywhere in your `main.tf` file:

```hcl
# --- Dotfiles

module "dotfiles" {
  count    = data.coder_workspace.me.start_count
  source   = "registry.coder.com/coder/dotfiles/coder"
  version  = "1.4.2"
  agent_id = coder_agent.example.id
}
```

Next, save the `main.tf` file,
and push a new version of your template:

```shell
coder templates push -d ~/coder-quickstart -y quickstart
```

Adding this module will show two parameters during workspace creation:

- **Dotfiles Branch**, the branch to use for the dotfiles repository.
- **Dotfiles URL**, the URL of the dotfiles repository.
  This module supports both SSH (`git@host:user/repo`)
  and HTTPS (`https://host/user/repo`) URLs.

For more information about dotfiles and best practices,
visit [Dotfiles](https://dotfiles.github.io/).

## Step 4: Add support for external Git authentication

The original Quickstart template works well for public repos,
but private repos require manual authentication from users.
Coder supports external authentication with OAuth 2.0
through the `coder_external_auth` data source in Terraform.

The following block adds support for GitHub external authentication:

```hcl
data "coder_external_auth" "github" {
  id = "github"
}
```

Don't forget to save your changes and
push a new version of your template:

push a new version of your template:

```shell
coder templates push -d ~/coder-quickstart -y quickstart
```

While this tutorial uses GitHub,
you can apply these steps to other providers.
For a full list of supported external authentication providers,
visit [External Authentication](../../admin/external-auth/index.md).

## Conclusion

Congratulations! You now updated your template to support an additional programming language, use dotfiles to customize workspace startup behavior, and added external authentication support for private repositories.

## What's next?

- Learn more about extending templates by using [parameters](../../admin/templates/extending-templates/parameters.md).
- Explore the collection of [templates on the Coder Registry](https://registry.coder.com/templates) for inspiration.
