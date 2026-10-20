---
title: Form types and styling
description: Choose the input control for each parameter, such as a dropdown, slider, multi-select, or text area.
---

The `form_type` and `styling` attributes of [`coder_parameter`](https://registry.terraform.io/providers/coder/coder/latest/docs/data-sources/parameter) control how a parameter renders in the workspace creation form.
Use them to pick the right control for each value, such as a searchable dropdown for a long list or a slider for a resource size.

These attributes require version 2.4.0 or later of the Coder Terraform provider.
Refer to [Coder provider version](./index.md#coder-provider-version) for more information.

## Form types

Different parameter types support different form types.
You can specify the form type using the [`form_type`](https://registry.terraform.io/providers/coder/coder/latest/docs/data-sources/parameter#form_type-1) attribute.
You can specify the form type with the [`form_type`](https://registry.terraform.io/providers/coder/coder/latest/docs/data-sources/parameter#form_type-1) attribute.
The **Options** column in the table below indicates whether the form type supports options (**Yes**) or doesn't support them (**No**).
When supported, you can specify options using one or more `option` blocks in your parameter definition, where each option has a `name` (displayed to the user) and a `value` (used in your template logic).

| Form Type      | Parameter Types                            | Options | Notes                                                                                                                   |
|----------------|--------------------------------------------|---------|-------------------------------------------------------------------------------------------------------------------------|
| `radio`        | `string`, `number`, `bool`, `list(string)` | Yes     | Radio buttons for selecting a single option with all choices visible at once. <br />The classic parameter option.       |
| `dropdown`     | `string`, `number`                         | Yes     | Choose a single option from a searchable dropdown list. <br />Default for `string` or `number` parameters with options. |
| `multi-select` | `list(string)`                             | Yes     | Select multiple items from a searchable dropdown list. <br />Selected items are shown as removable chips.               |
| `tag-select`   | `list(string)`                             | No      | Default for `list(string)` parameters without options.                                                                  |
| `input`        | `string`, `number`                         | No      | Standard single-line text input field. <br />Default for `string/number` parameters without options.                    |
| `textarea`     | `string`                                   | No      | Multi-line text input field for longer content.                                                                         |
| `slider`       | `number`                                   | No      | Slider selection with min/max validation for numeric values.                                                            |
| `checkbox`     | `bool`                                     | No      | A single checkbox for boolean parameters. <br />Default for boolean parameters.                                         |

## Styling options

The `coder_parameter` resource supports an additional `styling` attribute for special cosmetic changes that can be used to further customize the workspace creation form.

This can be used for:

- Masking private inputs.
- Marking inputs as read-only.
- Setting placeholder text.

Note that the `styling` attribute should not be used as a governance tool, since it only changes how the interactive form is displayed.
Users can avoid restrictions like `disabled` if they create a workspace via the CLI.

This attribute accepts JSON like so:

```tf
data "coder_parameter" "styled_parameter" {
  ...
  styling = jsonencode({
    disabled = true
  })
}
```

Not all styling attributes are supported by all form types, use the reference below for syntax:

| Styling Option | Compatible parameter types | Compatible form types | Notes                                                                               |
|----------------|----------------------------|-----------------------|-------------------------------------------------------------------------------------|
| `disabled`     | All parameter types        | All form types        | Disables the form control when `true`.                                              |
| `placeholder`  | `string`                   | `input`, `textarea`   | Sets placeholder text. <br />This is overwritten by user entry.                     |
| `mask_input`   | `string`, `number`         | `input`, `textarea`   | Masks inputs as asterisks (`*`). Used to cosmetically hide token or password entry. |

## Form type examples

The following examples show each [form type](#form-types) in use.

### Dropdowns

Single-select parameters with options can use the `form_type="dropdown"` attribute for better organization.

[Try dropdown lists on the Parameter Playground](https://playground.coder.app/parameters/kgNBpjnz7x)

```tf
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
  name = "ides_dropdown"
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

[Try `textarea` parameters on the Parameter Playground](https://playground.coder.app/parameters/RCAHA1Oi1_)

```tf

data "coder_parameter" "text_area" {
  name = "text_area"
  description  = "Enter multi-line text."
  mutable      = true
  display_name = "Textarea"

  form_type = "textarea"
  type      = "string"

  default = <<-EOT
    This is an example of multi-line text entry.

    The 'textarea' form_type is useful for
    - AI prompts
    - Scripts
    - Read-only info (try the 'disabled' styling option)
  EOT
}

```

### Multi-select

Multi-select parameters allow users to select one or many options from a single list of options.
For example, adding multiple IDEs with a single parameter.

[Try multi-select parameters on the Parameter Playground](https://playground.coder.app/parameters/XogX54JV_f)

```tf
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

### Radio

Radio buttons are used to select a single option with high visibility.
This is the original styling for list parameters.

[Try radio parameters on the Parameter Playground](https://playground.coder.app/parameters/3OMDp5ANZI).

```tf
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

A single checkbox for boolean values.
This can be used for a TOS confirmation or to expose advanced options.

[Try checkbox parameters on the Parameters Playground](https://playground.coder.app/parameters/ycWuQJk2Py).

```tf
data "coder_parameter" "enable_gpu" {
  name         = "enable_gpu"
  display_name = "Enable GPU"
  type         = "bool"
  form_type    = "checkbox" # This is the default for boolean parameters
  default      = false
}
```

### Slider

Sliders can be used for configuration on a linear scale, like resource allocation.
The `validation` block is used to constrain (or clamp) the minimum and maximum values for the parameter.

[Try slider parameters on the Parameters Playground](https://playground.coder.app/parameters/RsBNcWVvfm).

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

### Masked input

Masked input parameters can be used to visually hide secret values in the workspace creation form.
Note that this does not secure information on the backend and is purely cosmetic.

```tf
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
