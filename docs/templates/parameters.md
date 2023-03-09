# Parameters (alpha)

> Parameters are an [alpha feature](../contributing/feature-stages.md#alpha-features). See the [Rich Parameters Milestone](https://github.com/coder/coder/milestone/11) for more details.

Templates can contain _parameters_, which prompt the user for additional information
in the "create workspace" screen.

![Parameters in Create Workspace screen](../images/parameters.png)

```hcl
data "coder_parameter" "docker_host" {
  name        = "Region"
  description = "Which region would you like to deploy to?"
  icon        = "/emojis/1f30f.png"
  type        = "string"
  default     = "tcp://100.94.74.63:2375"

  option {
    name = "Pittsburgh, USA"
    value = "tcp://100.94.74.63:2375"
    icon = "/emojis/1f1fa-1f1f8.png"
  }

  option {
    name = "Helsinki, Finland"
    value = "tcp://100.117.102.81:2375"
    icon = "/emojis/1f1eb-1f1ee.png"
  }

  option {
    name = "Sydney, Australia"
    value = "tcp://100.127.2.1:2375"
    icon = "/emojis/1f1e6-1f1f9.png"
  }
}
```

From there, parameters can be referenced during build-time:

```hcl
provider "docker" {
  host = data.coder_parameter.docker_host.value
}
```

> For a complete list of supported parameter types, see the
> [coder_parameter Terraform reference](https://registry.terraform.io/providers/coder/coder/latest/docs/data-sources/parameter)

## Optional

A parameter is consider to be _required_ if it doesn't have the `default` property. It means that the workspace user needs to provide the parameter value before creating a workspace.

```hcl
data "coder_parameter" "account_name" {
  name        = "Account name"
  description = "Cloud account name"
  mutable     = true
}
```

If a parameter contains the `default` property, coder will use it when the workspace user doesn't specify the custom value. This way admins can set the `default` property to an empty value,
so that the parameter field can remain empty.

```hcl
data "coder_parameter" "dotfiles_url" {
  name        = "dotfiles URL"
  description = "Git repository with dotfiles"
  mutable     = true
  default     = ""
}
```

## Mutable

TODO

## Validation

TODO

## Migration

TODO

### Terraform variables

TODO

TODO sensitive

## Legacy

Prior to Coder v0.16.0 (Jan 2023), parameters were defined via Terraform `variable` blocks. These "legacy parameters" can still be used in templates, but will be removed in April 2023.

```hcl
variable "use_kubeconfig" {
  sensitive   = true # Admin (template-level) parameter
  type        = bool
  description = <<-EOF
  Use host kubeconfig? (true/false)
  EOF
}

variable "cpu" {
  sensitive   = false # User (workspace-level) parameter
  description = "CPU (__ cores)"
  default     = 2
  validation {
    condition = contains([
      "2",
      "4",
      "6",
      "8"
    ], var.cpu)
    error_message = "Invalid cpu!"
  }
}
```

> ⚠️ Legacy (`variable`) parameters and rich parameters cannot be used in the same template.
