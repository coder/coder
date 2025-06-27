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

You can specify the form type using the `form_type` property.
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

<tabs>

## Dropdown lists

All single-select parameters with options may now use the `form_type=\"dropdown\"` attribute for better organization.

```
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


## Large text entry

## Multi-select



</tabs>



### Conditional Parameters

<tabs>



</tabs>



<tabs>

## Admin Options

## Role-specific options

## 

 </tabs>


### Form Type Examples

<details><summary>checkbox: A single checkbox for boolean values</summary>

```tf
data "coder_parameter" "enable_gpu" {
  name         = "enable_gpu"
  display_name = "Enable GPU"
  type         = "bool"
  form_type    = "checkbox" # This is the default for boolean parameters
  default      = false
}
```

</details>

<details><summary>dropdown: A searchable select menu for choosing a single option from a list</summary>

```tf
data "coder_parameter" "region" {
  name         = "region"
  display_name = "Region"
  description  = "Select a region"
  type         = "string"
  form_type    = "dropdown" # This is the default for string parameters with options

  option {
    name  = "US East"
    value = "us-east-1"
  }
  option {
    name  = "US West"
    value = "us-west-2"
  }
}
```

</details>

<details><summary>input: A standard text input field</summary>

```tf
data "coder_parameter" "custom_domain" {
  name         = "custom_domain"
  display_name = "Custom Domain"
  type         = "string"
  form_type    = "input" # This is the default for string parameters without options
  default      = ""
}
```

</details>

<details><summary>multi-select: Checkboxes for selecting multiple options from a list</summary>

```tf
data "coder_parameter" "tools" {
  name         = "tools"
  display_name = "Developer Tools"
  type         = "list(string)"
  form_type    = "multi-select"
  default      = jsonencode(["git", "docker"])

  option {
    name  = "Git"
    value = "git"
  }
  option {
    name  = "Docker"
    value = "docker"
  }
  option {
    name  = "Kubernetes CLI"
    value = "kubectl"
  }
}
```

</details>

<details><summary>password: A text input that masks sensitive information</summary>

```tf
data "coder_parameter" "api_key" {
  name         = "api_key"
  display_name = "API Key"
  type         = "string"
  form_type    = "password"
  secret       = true
}
```

</details>

<details><summary>radio: Radio buttons for selecting a single option with high visibility</summary>

```tf
data "coder_parameter" "environment" {
  name         = "environment"
  display_name = "Environment"
  type         = "string"
  form_type    = "radio"
  default      = "dev"

  option {
    name  = "Development"
    value = "dev"
  }
  option {
    name  = "Staging"
    value = "staging"
  }
}
```

</details>

<details><summary>slider: A slider for selecting numeric values within a range</summary>

```tf
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

</details>

<details><summary>switch: A toggle switch for boolean values</summary>

```tf
data "coder_parameter" "advanced_mode" {
  name         = "advanced_mode"
  display_name = "Advanced Mode"
  type         = "bool"
  form_type    = "switch"
  default      = false
}
```

</details>

<details><summary>textarea: A multi-line text input field for longer content</summary>

```tf
data "coder_parameter" "init_script" {
  name         = "init_script"
  display_name = "Initialization Script"
  type         = "string"
  form_type    = "textarea"
  default      = "#!/bin/bash\necho 'Hello World'"
}
```

</details>

## Dynamic Parameter Use Case Examples

<details><summary>Conditional Parameters: Region and Instance Types</summary>

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

</details>

<details><summary>Advanced Options Toggle</summary>

This example shows how to create an advanced options section:

```tf
data "coder_parameter" "show_advanced" {
  name         = "show_advanced"
  display_name = "Show Advanced Options"
  description  = "Enable to show advanced configuration options"
  type         = "bool"
  default      = false
  order        = 0
}

data "coder_parameter" "advanced_setting" {
  # This parameter is only visible when show_advanced is true
  count = data.coder_parameter.show_advanced.value ? 1 : 0
  name         = "advanced_setting"
  display_name = "Advanced Setting"
  description  = "An advanced configuration option"
  type         = "string"
  default      = "default_value"
  mutable      = true
  order        = 1
}

</details>

<details><summary>Multi-select IDE Options</summary>

This example allows selecting multiple IDEs to install:

```tf
data "coder_parameter" "ides" {
  name         = "ides"
  display_name = "IDEs to Install"
  description  = "Select which IDEs to install in your workspace"
  type         = "list(string)"
  default      = jsonencode(["vscode"])
  mutable      = true
  form_type    = "multi-select"

  option {
    name  = "VS Code"
    value = "vscode"
    icon  = "/icon/vscode.png"
  }

  option {
    name  = "JetBrains IntelliJ"
    value = "intellij"
    icon  = "/icon/intellij.png"
  }

  option {
    name  = "JupyterLab"
    value = "jupyter"
    icon  = "/icon/jupyter.png"
  }
}
```

</details>

<details><summary>Team-specific Resources</summary>

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

### Advanced Usage Patterns

<details><summary>Creating Branching Paths</summary>

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

</details>

<details><summary>Conditional Validation</summary>

Adjust validation rules dynamically based on parameter values:

```tf
data "coder_parameter" "team" {
  name        = "team"
  display_name = "Team"
  type        = "string"
  default     = "engineering"

  option {
    name  = "Engineering"
    value = "engineering"
  }

  option {
    name  = "Data Science"
    value = "data-science"
  }
}

data "coder_parameter" "cpu_count" {
  name        = "cpu_count"
  display_name = "CPU Count"
  type        = "number"
  default     = 2

  # Engineering team has lower limits
  dynamic "validation" {
    for_each = data.coder_parameter.team.value == "engineering" ? [1] : []
    content {
      min = 1
      max = 4
    }
  }

  # Data Science team has higher limits
  dynamic "validation" {
    for_each = data.coder_parameter.team.value == "data-science" ? [1] : []
    content {
      min = 2
      max = 8
    }
  }
}
```

</details>
