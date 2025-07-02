# Dynamic Parameters (Beta)

Coder v2.24.0 introduces Dynamic Parameters to extend the existing parameter system with conditional form controls, enriched input types, and user idenitity awareness.
This feature allows template authors to create interactive workspace creation forms, meaning more environment customization and fewer templates to maintain.

All parameters are parsed from Terraform, meaning your workspace creation forms live in the same location as your provisioning code. You can use all the native Terraform functions and conditionality to create a self-service tooling catalog for every template.

Administrators can now:

- Create parameters which respond to the inputs of others
- Only show parameters when other input criteria is met
- Only show select parameters to target Coder roles or groups

You can try the dynamic parameter syntax and any of the code examples below in the [Parameters Playground](https://playground.coder.app/parameters) today. We advise experimenting here before upgrading templates.

### When you should upgrade to Dynamic Parameters

While Dynamic parameters introduce a variety of new powerful tools, all functionality is **backwards compatible** with existing coder templates. When opting-in to the new experience, no functional changes will be applied to your production parameters.

[Screenshot of before and after dynamic parameters on a "legacy" template]

There are three reasons for users to try Dynamic Parameters:

- You maintain support many templates for teams with unique expectations or use cases
- You want to selectively expose privledged workspace options to admins, power users, or personas
- You want to make the workspace creation flow more ergonomic for developers.

Dynamic Parameters help you reduce template duplication by setting conditions on which users may see which parameters. They increase the potential complexity of user-facing configuration by allowing administrators to organize a long list of options into interactive, branching paths for workspace customization. They allow you to set resource guardrails by referencing coder identity in the `coder_workspace_owner` data source.

Read on for setup steps and code examples.

### How to enable Dynamic Parameters

In v2.24.0, you can opt-in to Dynamic Parameters on a per-template basis. To use dynamic parameters, go to your template settings and toggle the "Enable Dynamic Parameters Beta" option.

[Image of template settings with dynamic parameters beta option]

Next, update your template to use version >=2.4.0 of the Coder provider with the following Terraform block.

```terraform
terraform {
  required_providers {
    coder = {
      source = "coder/coder"
      version = ">=2.4.0"
    }
  }
}
```

Once configured, users should see the updated workspace creation form. Then you are ready to start leveraging the new conditionality system and new input types. Note that these new features must be declared in your Terraform to start leveraging Dynamic Parameters. Note that dynamic parameters is backwards compatible, so all existing templates may be upgraded in-place.

If you decide later to revert to the legacy flow, Dynamic Parameters may be disabled in the template settings.

## Features and Capabilities

Dynamic Parameters introduces three primary enhancements to the standard parameter system:

- **Conditional Parameters**

  - Parameters can respond to changes in other parameters
  - Show or hide parameters based on other selections
  - Modify validation rules conditionally
  - Create branching paths in workspace creation forms

- **Reference User Properties**

  - Read user data at build time from [`coder_workspace_owner`](https://registry.terraform.io/providers/coder/coder/latest/docs/data-sources/workspace_owner)
  - Conditionally hide parameters based on user's role
  - Change parameter options based on user groups
  - Reference user name, groups, and roles in parameter text

- **Additional Form Inputs**

  - Searchable dropdown lists for easier selection
  - Multi-select options for choosing multiple items
  - Secret text inputs for sensitive information
  - Slider input for disk size, model temperature
  - Disabled parameters to display immutable data

## Available Form Input Types

Dynamic Parameters supports a variety of form types to create rich, interactive user experiences.

You can specify the form type using the [`form_type`](https://registry.terraform.io/providers/coder/coder/latest/docs/data-sources/parameter#form_type-1) attribute.
Different parameter types support different form types.

The "Options" column in the table below indicates whether the form type requires options to be defined (Yes) or doesn't support/require them (No). When required, options are specified using one or more `option` blocks in your parameter definition, where each option has a `name` (displayed to the user) and a `value` (used in your template logic).

| Form Type      | Parameter Types                            | Options | Notes                                                                                                                        |
|----------------|--------------------------------------------|---------|------------------------------------------------------------------------------------------------------------------------------|
| `checkbox`     | `bool`                                     | No      | A single checkbox for boolean parameters. Default for boolean parameters.                                                    |
| `dropdown`     | `string`, `number`                         | Yes     | Searchable dropdown list for choosing a single option from a list. Default for `string` or `number` parameters with options. |
| `input`        | `string`, `number`                         | No      | Standard single-line text input field. Default for string/number parameters without options.                                 |
| `multi-select` | `list(string)`                             | Yes     | Select multiple items from a list with checkboxes.                                                                           |
| `radio`        | `string`, `number`, `bool`, `list(string)` | Yes     | Radio buttons for selecting a single option with all choices visible at once.                                                |
| `slider`       | `number`                                   | No      | Slider selection with min/max validation for numeric values.                                                                 |
| `switch`       | `bool`                                     | No      | Toggle switch alternative for boolean parameters.                                                                            |
| `tag-select`   | `list(string)`                             | No      | Default for list(string) parameters without options.                                                                         |
| `textarea`     | `string`                                   | No      | Multi-line text input field for longer content.                                                                              |
| `error`        |                                            | No      | Used to display an error message when a parameter  form_type is unknown                                                      |

## Use case examples

### New Form Types

The following examples show some basic usage of the [`form_type`](https://registry.terraform.io/providers/coder/coder/latest/docs/data-sources/parameter#form_type-1) attribute explained above. These are used to change the input style of form controls in the create workspace form.

<div class="tabs">

### Dropdowns

Single-select parameters with options may now use the `form_type=\"dropdown\"` attribute for better organization.

[Try dropdown lists on the Parameter Playground](https://playground.coder.app/parameters/kgNBpjnz7x)

```terraform
locals {
  ides = [
    "VS Code",
    "JetBrains IntelliJ",
    "PyCharm",
    "GoLand",
    "WebStorm",
    "Vim",
    "Emacs",
    "Neovim"
  ]
}

data "coder_parameter" "ides_dropdown" {
  name = "ides_dropdow"
  display_name = "Select your IDEs"
  type = "string"

  form_type = "dropdown"

  dynamic "option" {
    for_each = local.ides
    content {
      name  = option.value
      value = option.value
    }
  }
}
```

### Text area

The large text entry option can be used to enter long strings like AI prompts, scripts, or natural language.

[Try textarea parameters on the Parameter Playground](https://playground.coder.app/parameters/RCAHA1Oi1_)

```terraform

data "coder_parameter" "text_area" {
  name = "text_area"
  description  = "Enter mutli-line text."
  mutable      = true
  display_name = "Select mutliple IDEs"

  form_type = "textarea"
  type      = "string"

  default = <<-EOT
    This is an example of mult-line text entry.

    The 'textarea' form_type is useful for
    - AI prompts
    - Scripts
    - Read-only info (try the 'disabled' styling option)
  EOT
}

```

### Multi-select

Multi-select parameters allow users to select one or many options from a single list of options. For example, adding multiple IDEs with a single parameter.

[Try multi-select parameters on the Parameter Playground](https://playground.coder.app/parameters/XogX54JV_f)

```terraform
locals {
  ides = [
    "VS Code", "JetBrains IntelliJ",
    "GoLand", "WebStorm",
    "Vim", "Emacs",
    "Neovim", "PyCharm",
    "Databricks", "Jupyter Notebook",
  ]
}

data "coder_parameter" "ide_selector" {
  name = "ide_selector"
  description  = "Choose any IDEs for your workspace."
  mutable      = true
  display_name = "Select multiple IDEs"


  # Allows users to select multiple IDEs from the list.
  form_type = "multi-select"
  type      = "list(string)"


  dynamic "option" {
    for_each = local.ides
    content {
      name  = option.value
      value = option.value
    }
  }
}
```

### Radio (classic)

Radio buttons are used to select a single option with high visibility. This is the original styling for list parameters.

[Try radio parameters on the Parameter Playground](https://playground.coder.app/parameters/3OMDp5ANZI).

```terraform
data "coder_parameter" "environment" {
  name         = "environment"
  display_name = "Environment"
  description  = "An example of environment listing with the radio form type."
  type         = "string"
  default      = "dev"

  form_type    = "radio"

  option {
    name  = "Development"
    value = "dev"
  }
  option {
    name  = "Experimental"
    value = "exp"
  }
  option {
    name  = "Staging"
    value = "staging"
  }
  option {
    name  = "Production"
    value = "prod"
  }
}
```

### Checkboxes

Checkbox: A single checkbox for boolean values

[Try checkbox parameters on the Parameters Playground](https://playground.coder.app/parameters/ycWuQJk2Py).

```terraform
data "coder_parameter" "enable_gpu" {
  name         = "enable_gpu"
  display_name = "Enable GPU"
  type         = "bool"
  form_type    = "checkbox" # This is the default for boolean parameters
  default      = false
}
```

### Slider

Sliders can be used for configuration on a linear scale, like resource allocation. The `validation` block is used to clamp the minimum and maximum values for the parameter.

[Try slider parameters on the Parameters Playground](https://playground.coder.app/parameters/RsBNcWVvfm).

```terraform
data "coder_parameter" "cpu_cores" {
  name         = "cpu_cores"
  display_name = "CPU Cores"
  type         = "number"
  form_type    = "slider"
  default      = 2
  validation {
    min = 1
    max = 8
  }
}
```

### Secrets

Sliders can be used for configuration on a linear scale, like resource allocation. The `validation` block is used to clamp the minimum and maximum values for the parameter.

[Try secret parameters on the Parameters Playground](https://playground.coder.app/parameters/wmiP7FM3Za).

Note: this text may not be properly hidden in the Playground. The `mask_input` styling attribute is supported in v2.24.0 and onward.

```terraform
data "coder_parameter" "private_api_key" {
  name         = "private_api_key"
  display_name = "Your super secret API key"
  type         = "string"
    
  form_type = "input" # | "textarea"

  # Will render as "**********"
  default = "privatekey"

  styling = jsonencode({
    mask_input = true
  })
}
```


</div>

### Conditional Parameters

Using native Terraform syntax and parameter attributes like `count`, we can allow some parameters to react to user inputs. This means:

- Hiding parameters unless activated
- Conditionally setting default values
- Changing available options based on other parameter inputs

Using these in conjunction, administrators can build intuitive, reactive forms for workspace creation

<div class="tabs">

### Hide/show options

Using Terraform conditionals and the `count` block, we can allow a checkbox to expose or hide a subsequent parameter.

[Try conditional parameters on the Parameter Playground](https://playground.coder.app/parameters/xmG5MKEGNM).

```terraform
data "coder_parameter" "show_cpu_cores" {
  name         = "show_cpu_cores"
  display_name = "Toggles next parameter"
  description  = "Select this checkbox to show the CPU cores parameter."
  type         = "bool"
  form_type    = "checkbox"
  default      = false
  order        = 1
}

data "coder_parameter" "cpu_cores" {
  # Only show this parameter if the previous box is selected.
  count = data.coder_parameter.show_cpu_cores.value ? 1 : 0

  name         = "cpu_cores"
  display_name = "CPU Cores"
  type         = "number"
  form_type    = "slider"
  default      = 2
  order        = 2
  validation {
    min = 1
    max = 8
  }
}
```

### Dynamic Defaults

For a given parameter, we can influence which option is selected by default based on the selection of another. This allows us to suggest an option dynamically without strict enforcement.

[Try dynamic defaults in the Parameter Playground](https://playground.coder.app/parameters/DEi-Bi6DVe).

```terraform
locals {
  ides = [
    "VS Code",
    "IntelliJ", "GoLand",
    "WebStorm", "PyCharm",
    "Databricks", "Jupyter Notebook",
  ]
  mlkit_ides = jsonencode(["Databricks", "PyCharm"])
  core_ides = jsonencode(["VS Code", "GoLand"])
}

data "coder_parameter" "git_repo" {
  name = "git_repo"
  display_name = "Git repo"
  description = "Select a git repo to work on."
  order = 1
  mutable = true
  type = "string"
  form_type = "dropdown"

  option {
    # A Go-heavy repository
    name = "coder/coder"
    value = "coder/coder"
  }

  option {
    # A python-heavy repository
    name = "coder/mlkit"
    value = "coder/mlkit"
  }
}

data "coder_parameter" "ide_selector" {
  # Conditionally expose this parameter
  count = try(data.coder_parameter.git_repo.value, "") != "" ? 1 : 0

  name = "ide_selector"
  description  = "Choose any IDEs for your workspace."
  order        = 2
  mutable      = true

  display_name = "Select IDEs"
  form_type = "multi-select"
  type      = "list(string)"
  default   = try(data.coder_parameter.git_repo.value, "") == "coder/mlkit" ? local.mlkit_ides : local.core_ides


  dynamic "option" {
    for_each = local.ides
    content {
      name  = option.value
      value = option.value
    }
  }
}
```

## Dynamic Validation

Parameters' validation block can also leverage inputs from other parameters.

[Try dynamic validation in the Parameter Playground](https://playground.coder.app/parameters/sdbzXxagJ4).

```terraform
data "coder_parameter" "git_repo" {
  name = "git_repo"
  display_name = "Git repo"
  description = "Select a git repo to work on."
  order = 1
  mutable = true
  type = "string"
  form_type = "dropdown"

  option {
    # A Go-heavy repository
    name = "coder/coder"
    value = "coder/coder"
  }

  option {
    # A python-heavy repository
    name = "coder/mlkit"
    value = "coder/mlkit"
  }
}

data "coder_parameter" "cpu_cores" {
  # Only show this parameter if the previous box is selected.
  count = data.coder_parameter.show_cpu_cores.value ? 1 : 0

  name         = "cpu_cores"
  display_name = "CPU Cores"
  type         = "number"
  form_type    = "slider"
  order        = 2

  # Dynamically set default
  default      = try(data.coder_parameter.git_repo.value, "") == "coder/mlkit" ? 12 : 6

  validation {
    min = 1

    # Dynamically set max validation
    max = try(data.coder_parameter.git_repo.value, "") == "coder/mlkit" ? 16 : 8
  }
}
```

## Daisy Chaining

You can daisy-chain the conditionals shown here to create a dynamically expanding form. Note that parameters must be indexed when using the `count` attribute. 

```terraform

data "coder_parameter" "git_repo" {
  name = "git_repo"
  display_name = "Git repo"
  description = "Select a git repo to work on."
  order = 1
  mutable = true
  type = "string"
  form_type = "dropdown"

  option {
    # A Go-heavy repository
    name = "coder/coder"
    value = "coder/coder"
  }

  option {
    # A python-heavy repository
    name = "coder/mlkit"
    value = "coder/mlkit"
  }
}

data "coder_parameter" "ide_selector" {
  # Conditionally expose this parameter
  count = try(data.coder_parameter.git_repo.value, "") != "" ? 1 : 0

  name = "ide_selector"
  description  = "Choose any IDEs for your workspace."
  order        = 2
  mutable      = true

  display_name = "Select IDEs"
  form_type = "multi-select"
  type      = "list(string)"
  default   = try(data.coder_parameter.git_repo.value, "") == "coder/mlkit" ? local.mlkit_ides : local.core_ides


  dynamic "option" {
    for_each = local.ides
    content {
      name  = option.value
      value = option.value
    }
  }
}

data "coder_parameter" "region" {
  # Only show this parameter if any IDEs are selected.
  count = try(data.coder_parameter.ide_selector.value,) ? 1 : 0

  name         = "region"
  display_name = "Select a Region"
  type         = "number"
  form_type    = "slider"
  order        = 2

  # Dynamically set default
  default      = try(data.coder_parameter.git_repo.value, "") == "coder/mlkit" ? 12 : 6

  validation {
    min = 1

    # Dynamically set max validation
    max = try(data.coder_parameter.git_repo.value, "") == "coder/mlkit" ? 16 : 8
  }
}

```

</div>

## Identity-aware parameters (Premium)

Premium users can leverage our roles and groups to conditionally expose or change parameters based on user identity. This is helpful for establishing governance policy directly in the workspace creation form, rather than creating multiple templates to manage RBAC.

User identity is referenced in Terraform by reading the [`coder_workspace_owner`](https://registry.terraform.io/providers/coder/coder/latest/docs/data-sources/workspace_owner) data source.

<div class="tabs">

## Admin Options

Template administrators often want to expose certain experimental or unstable options only to those with elevated roles. You can now do this by setting `count` based on a user's group or role, referencing the [`coder_workspace_owner`](https://registry.terraform.io/providers/coder/coder/latest/docs/data-sources/workspace_owner) data source. 

[Try out admin-only options in the Playground](https://playground.coder.app/parameters/5Gn9W3hYs7).

```terraform

locals {
  roles = [for r in data.coder_workspace_owner.me.rbac_roles: r.name]
  is_admin = contains(data.coder_workspace_owner.me.groups, "admin")
  has_admin_role = contains(local.roles, "owner")
}

data "coder_workspace_owner" "me" {}

data "coder_parameter" "advanced_settings" {
  # This parameter is only visible when the user is an administrator
  count = local.is_admin ? 1 : 0
  
  name         = "advanced_settings"
  display_name = "Add an arbitrary script"
  description  = "An advanced configuration option only available to admins."
  type         = "string"
  form_type    = "textarea" 
  mutable      = true
  order        = 5

  styling = jsonencode({
    placeholder = <<-EOT
  #!/usr/bin/env bash
  while true; do
    echo "hello world"
    sleep 1
  done
  EOT
  })
}

```

## Role-specific options

Similarly to the above example, you can show certain options to specific roles on the platform. This allows you to restrict resources for those who may not need them.

```terraform
locals {
  roles = [for r in data.coder_workspace_owner.me.rbac_roles: r.name]
  is_admin = contains(data.coder_workspace_owner.me.groups, "admin")
  has_admin_role = contains(roles, "owner")



  non_admin_desc = "YOU ARE NOT AN ADMIN! Set user to 'Administrator' in the top right."
  admin_desc     = "You are an administrator! You now have access to the secret advanced option."
}

data "coder_workspace_owner" "me" {}

data "coder_parameter" "advanced_settings" {
  # This parameter is only visible when show_advanced is true
  count = local.is_admin ? 1 : 0
  name         = "advanced_settings"
  display_name = "Add an arbitrary script"
  description  = "An advanced configuration option only available to admins."
  type         = "string"
  form_type    = "textarea" 

  styling = jsonencode({
    placeholder = <<-EOT
  #!/usr/bin/env bash
  while true; do
    echo "hello world"
    sleep 1
  done
  EOT
  })

  mutable      = true
  order        = 5
}
```

### User-aware Regions

You can expose regions depending on which group a user belongs to. This way developers can't incidentally induce low-latency with world-spanning connections. 

[Try user-aware regions in the parameter playground](https://playground.coder.app/parameters/tBD-mbZRGm)

```terraform

locals {
  eu_regions = [
    "eu-west-1 (Ireland)",
    "eu-central-1 (Frankfurt)",
    "eu-north-1 (Stockholm)",
    "eu-west-3 (Paris)",
    "eu-south-1 (Milan)"
  ]

  us_regions = [
    "us-east-1 (N. Virginia)",
    "us-west-1 (California)",
    "us-west-2 (Oregon)",
    "us-east-2 (Ohio)",
    "us-central-1 (Iowa)"
  ]

  eu_group_name = "eu-helsinki"
  is_eu_dev = contains(data.coder_workspace_owner.me.groups, local.eu_group_name)
  region_desc_tag = local.is_eu_dev ? "european" : "american"
}

data "coder_parameter" "region" {
  name         = "region"
  display_name = "Select a Region"
  description  = "Select from ${local.region_desc_tag} region options."
  type         = "string"
  form_type    = "dropdown"
  order        = 5
  default      = local.is_eu_dev ? local.eu_regions[0] : local.us_regions[0]

  dynamic "option" {
    for_each = local.is_eu_dev ? local.eu_regions : local.us_regions
    content {
      name        = option.value
      value       = option.value
      description = "Use ${option.value}"
    }
  }
}
```


### Groups as namespaces

A slightly unorthodox way to leverage this is filling the selections of a parameter from the user's groups. Some users associate groups with namespaces (E.G. Kubernetes), then allow users to target that namespace with a parameter like so.


```terraform


```

</div>

This example shows instance types based on the selected region:

```tf
data "coder_parameter" "region" {
  name        = "region"
  display_name = "Region"
  description = "Select a region for your workspace"
  type        = "string"
  default     = "us-east-1"

  option {
    name  = "US East (N. Virginia)"
    value = "us-east-1"
  }

  option {
    name  = "US West (Oregon)"
    value = "us-west-2"
  }
}

data "coder_parameter" "instance_type" {
  name         = "instance_type"
  display_name = "Instance Type"
  description  = "Select an instance type available in the selected region"
  type         = "string"

  # This option will only appear when us-east-1 is selected
  dynamic "option" {
    for_each = data.coder_parameter.region.value == "us-east-1" ? [1] : []
    content {
      name  = "t3.large (US East)"
      value = "t3.large"
    }
  }

  # This option will only appear when us-west-2 is selected
  dynamic "option" {
    for_each = data.coder_parameter.region.value == "us-west-2" ? [1] : []
    content {
      name  = "t3.medium (US West)"
      value = "t3.medium"
    }
  }
}
```

This example filters resources based on user group membership:

```tf
data "coder_parameter" "instance_type" {
  name        = "instance_type"
  display_name = "Instance Type"
  description = "Select an instance type for your workspace"
  type        = "string"

  # Show GPU options only if user belongs to the "data-science" group
  dynamic "option" {
    for_each = contains(data.coder_workspace_owner.me.groups, "data-science") ? [1] : []
    content {
      name  = "p3.2xlarge (GPU)"
      value = "p3.2xlarge"
    }
  }

  # Standard options for all users
  option {
    name  = "t3.medium (Standard)"
    value = "t3.medium"
  }
}
```

### 

For templates serving multiple teams or use cases, you can create comprehensive branching paths:

```tf
data "coder_parameter" "environment_type" {
  name         = "environment_type"
  display_name = "Environment Type"
  description  = "Select your preferred development environment"
  type         = "string"
  default      = "container"

  option {
    name  = "Container"
    value = "container"
  }

  option {
    name  = "Virtual Machine"
    value = "vm"
  }
}

# Container-specific parameters
data "coder_parameter" "container_image" {
  name         = "container_image"
  display_name = "Container Image"
  description  = "Select a container image for your environment"
  type         = "string"
  default      = "ubuntu:latest"

  # Only show when container environment is selected
  condition {
    field = data.coder_parameter.environment_type.name
    value = "container"
  }

  option {
    name  = "Ubuntu"
    value = "ubuntu:latest"
  }

  option {
    name  = "Python"
    value = "python:3.9"
  }
}

# VM-specific parameters
data "coder_parameter" "vm_image" {
  name         = "vm_image"
  display_name = "VM Image"
  description  = "Select a VM image for your environment"
  type         = "string"
  default      = "ubuntu-20.04"

  # Only show when VM environment is selected
  condition {
    field = data.coder_parameter.environment_type.name
    value = "vm"
  }

  option {
    name  = "Ubuntu 20.04"
    value = "ubuntu-20.04"
  }

  option {
    name  = "Debian 11"
    value = "debian-11"
  }
}
```

## Troubleshooting

