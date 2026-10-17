---
title: Conditional parameters
---

Conditional parameters react to the user's other selections in the workspace creation form.
Use native Terraform syntax and parameter attributes like `count` to:

- Hide parameters unless the user activates them.
- Set default values based on other selections.
- Change the available options based on other parameter inputs.

Combine these patterns to build an intuitive form that branches for each use case, so you maintain one template instead of several.

![Conditional parameters in the workspace creation form](../../../../images/admin/templates/extend-templates/dyn-params/dynamic-parameters-in-action.gif)

To make a parameter's validation rules depend on other selections, refer to [Validation that depends on other parameters](./validation.md#validation-that-depends-on-other-parameters).
To change parameters based on who is filling in the form, refer to [Identity-aware parameters](./identity-aware-parameters.md).

## Show or hide a parameter

Use Terraform conditionals and the `count` block to allow a checkbox to expose or hide a subsequent parameter.

[Try conditional parameters on the Parameter Playground](https://playground.coder.app/parameters/xmG5MKEGNM).

```tf
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

## Set defaults from other parameters

Influence which option is selected by default for one parameter based on the selection of another.
This allows you to suggest an option dynamically without strict enforcement.

[Try dynamic defaults in the Parameter Playground](https://playground.coder.app/parameters/DEi-Bi6DVe).

```tf
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

<!-- ## Daisy Chaining

You can daisy-chain the conditionals shown here to create a dynamically expanding form.
Note that parameters must be indexed when using the `count` attribute.

[Try daisy-chaining parameters in the Parameter Playground](https://playground.coder.app/parameters/jLUUhoDLIa).

```tf

locals {
  ides = [
    "VS Code",
    "JetBrains IntelliJ",
    "GoLand",
    "WebStorm",
    "PyCharm",
    "Databricks",
    "Jupyter Notebook",
  ]

  is_ml_repo = data.coder_parameter.git_repo == "coder/mlkit"

  selected = jsondecode(data.coder_parameter.ide_selector[0].value)

  # selected = try(jsondecode(data.coder_parameter.ide_selector[0].value), [])
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
    name = "coder/coder"
    value = "coder/coder"
  }

  option {
    name = "coder/mlkit"
    value = "coder/mlkit"
  }
}

data "coder_parameter" "ide_selector" {
  # Only show this parameter if a git repo has been selected.
  count = try(data.coder_parameter.git_repo.value, "") != "" ? 1 : 0
  name = "ide_selector"
  description  = "Choose any IDEs for your workspace."
  mutable      = true
  display_name = "Select multiple IDEs"
  order = 1
  default = "[]"

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

data "coder_parameter" "cpu_cores" {
  # Only show this parameter if the IDEs have been selected.
  count = length(local.selected) > 0 ? 1 : 0

  name         = "cpu_cores"
  display_name = "CPU Cores"
  type         = "number"
  form_type    = "slider"
  default      = local.is_ml_repo ? 12 : 6
  order        = 2
  validation {
    min = 1
    max = local.is_ml_repo ? 16 : 8
  }
}
``` -->
