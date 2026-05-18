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

After line 71, add the following text, matching the indentation of the text around it:

```hcl
  option {
    name  = "Ruby"
    value = "ruby"
    icon  = "/icon/ruby.svg"
  }
```

Save the file and publish the new version of the template:

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

</details>
