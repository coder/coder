# Parameters

A template can prompt the user for additional information when creating
workspaces with
[_parameters_](https://registry.terraform.io/providers/coder/coder/latest/docs/data-sources/parameter).

![Parameters in Create Workspace screen](../../../images/parameters.png)

The user can set parameters in the dashboard UI and CLI.

You'll likely want to hardcode certain template properties for workspaces, such
as security group. But you can let developers specify other properties with
parameters like instance size, geographical location, repository URL, etc.

This example lets a developer choose a Docker host for the workspace:

```tf
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

From there, a template can refer to a parameter's value:

```tf
provider "docker" {
  host = data.coder_parameter.docker_host.value
}
```

## Types

A Coder parameter can have one of these types:

- `string`
- `bool`
- `number`
- `list(string)`

To specify a default value for a parameter with the `list(string)` type, use a
JSON array and the Terraform
[jsonencode](https://developer.hashicorp.com/terraform/language/functions/jsonencode)
function. For example:

```tf
data "coder_parameter" "security_groups" {
  name        = "Security groups"
  icon        = "/icon/aws.png"
  type        = "list(string)"
  description = "Select appropriate security groups."
  mutable     = true
  default = jsonencode([
    "Web Server Security Group",
    "Database Security Group",
    "Backend Security Group"
  ])
}
```

> [!NOTE]
> Overriding a `list(string)` on the CLI is tricky because:
>
> - `--parameter "parameter_name=parameter_value"` is parsed as CSV.
> - `parameter_value` is parsed as JSON.
>
> So, to properly specify a `list(string)` with the `--parameter` CLI argument,
> you will need to take care of both CSV quoting and shell quoting.
>
> For the above example, to override the default values of the `security_groups`
> parameter, you will need to pass the following argument to `coder create`:
>
> ```shell
> --parameter "\"security_groups=[\"\"DevOps Security Group\"\",\"\"Backend Security Group\"\"]\""
> ```
>
> Alternatively, you can use `--rich-parameter-file` to work around the above
> issues. This allows you to specify parameters as YAML. An equivalent parameter
> file for the above `--parameter` is provided below:
>
> ```yaml
> security_groups:
>   - DevOps Security Group
>   - Backend Security Group
> ```

## Options

A `string` parameter can provide a set of options to limit the user's choices:

```tf
data "coder_parameter" "docker_host" {
  name        = "Region"
  description = "Which region would you like to deploy to?"
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

### Incompatibility in Parameter Options for Workspace Builds

When creating Coder templates, authors have the flexibility to modify parameter
options associated with rich parameters. Such modifications can involve adding,
substituting, or removing a parameter option. It's important to note that making
these changes can lead to discrepancies in parameter values utilized by ongoing
workspace builds.

Consequently, workspace users will be prompted to select the new value from a
pop-up window or by using the command-line interface. While this additional
interactive step might seem like an interruption, it serves a crucial purpose.
It prevents workspace users from becoming trapped with outdated template
versions, ensuring they can smoothly update their workspace without any
hindrances.

Example:

- Bob creates a workspace using the `python-dev` template. This template has a
  parameter `image_tag`, and Bob selects `1.12`.
- Later, the template author Alice is notified of a critical vulnerability in a
  package installed in the `python-dev` template, which affects the image tag
  `1.12`.
- Alice remediates this vulnerability, and pushes an updated template version
  that replaces option `1.12` with `1.13` for the `image_tag` parameter. She
  then notifies all users of that template to update their workspace
  immediately.
- Bob saves their work, and selects the `Update` option in the UI. As their
  workspace uses the now-invalid option `1.12`, for the `image_tag` parameter,
  they are prompted to select a new value for `image_tag`.

## Required and optional parameters

A parameter is _required_ if it doesn't have the `default` property. The user
**must** provide a value to this parameter before creating a workspace:

```tf
data "coder_parameter" "account_name" {
  name        = "Account name"
  description = "Cloud account name"
  mutable     = true
}
```

If a parameter contains the `default` property, Coder will use this value if the
user does not specify any:

```tf
data "coder_parameter" "base_image" {
  name        = "Base image"
  description = "Base machine image to download"
  default     = "ubuntu:latest"
}
```

Admins can also set the `default` property to an empty value so that the
parameter field can remain empty:

```tf
data "coder_parameter" "dotfiles_url" {
  name        = "dotfiles URL"
  description = "Git repository with dotfiles"
  mutable     = true
  default     = ""
}
```

## Mutability

Immutable parameters can only be set in these situations:

- Creating a workspace for the first time.
- Updating a workspace to a new template version. This sets the initial value
  for required parameters.

The idea is to prevent users from modifying fragile or persistent workspace
resources like volumes, regions, and so on.

Example:

```tf
data "coder_parameter" "region" {
  name        = "Region"
  description = "Region where the workspace is hosted"
  mutable     = false
  default     = "us-east-1"
}
```

You can modify a parameter's `mutable` attribute state anytime. In case of
emergency, you can temporarily allow for changing immutable parameters to fix an
operational issue, but it is not advised to overuse this opportunity.

## Ephemeral parameters

Ephemeral parameters are introduced to users in order to model specific
behaviors in a Coder workspace, such as reverting to a previous image, restoring
from a volume snapshot, or building a project without using cache. These
parameters are only settable when starting, updating, or restarting a workspace
and do not persist after the workspace is stopped.

Since these parameters are ephemeral in nature, subsequent builds proceed in the
standard manner:

```tf
data "coder_parameter" "force_rebuild" {
  name         = "force_rebuild"
  type         = "bool"
  description  = "Rebuild the Docker image rather than use the cached one."
  mutable      = true
  default      = false
  ephemeral    = true
}
```

## Validating parameters

Coder supports rich parameters with multiple validation modes: min, max,
monotonic numbers, and regular expressions.

### Number

You can limit a `number` parameter to `min` and `max` boundaries.

You can also specify its monotonicity as `increasing` or `decreasing` to verify
the current and new values. Use the `monotonic` attribute for resources that
can't be shrunk or grown without implications, like disk volume size.

```tf
data "coder_parameter" "instances" {
  name        = "Instances"
  type        = "number"
  description = "Number of compute instances"
  validation {
    min       = 1
    max       = 8
    monotonic = "increasing"
  }
}
```

It is possible to override the default `error` message for a `number` parameter,
along with its associated `min` and/or `max` properties. The following message
placeholders are available `{min}`, `{max}`, and `{value}`.

```tf
data "coder_parameter" "instances" {
  name        = "Instances"
  type        = "number"
  description = "Number of compute instances"
  validation {
    min       = 1
    max       = 4
    error     = "Sorry, we can't provision too many instances - maximum limit: {max}, wanted: {value}."
  }
}
```

> [!NOTE]
> As of
> [`terraform-provider-coder` v0.19.0](https://registry.terraform.io/providers/coder/coder/0.19.0/docs),
> `options` can be specified in `number` parameters; this also works with
> validations such as `monotonic`.

### String

You can validate a `string` parameter to match a regular expression. The `regex`
property requires a corresponding `error` property.

```tf
data "coder_parameter" "project_id" {
  name        = "Project ID"
  description = "Alpha-numeric project ID"
  validation {
    regex = "^[a-z0-9]+$"
    error = "Unfortunately, this isn't a valid project ID"
  }
}
```

## Workspace presets (beta)

Workspace presets allow you to configure commonly used combinations of parameters
into a single option, which makes it easier for developers to pick one that fits
their needs.

![Template with options in the preset dropdown](../../../images/admin/templates/extend-templates/template-preset-dropdown.png)

Use `coder_workspace_preset` to define the preset parameters.
After you save the template file, the presets will be available for all new
workspace deployments.

<details><summary>Expand for an example</summary>

```tf
data "coder_workspace_preset" "goland-gpu" {
  name        = "GoLand with GPU"
  parameters = {
    "machine_type"  = "n1-standard-1"
    "attach_gpu"    = "true"
    "gcp_region"    = "europe-west4-c"
    "jetbrains_ide" = "GO"
  }
}

data "coder_parameter" "machine_type" {
  name          = "machine_type"
  display_name  = "Machine Type"
  type          = "string"
  default       = "n1-standard-2"
}

data "coder_parameter" "attach_gpu" {
  name          = "attach_gpu"
  display_name  = "Attach GPU?"
  type          = "bool"
  default       = "false"
}

data "coder_parameter" "gcp_region" {
  name          = "gcp_region"
  display_name  = "Machine Type"
  type          = "string"
  default       = "n1-standard-2"
}

data "coder_parameter" "jetbrains_ide" {
  name          = "jetbrains_ide"
  display_name  = "Machine Type"
  type          = "string"
  default       = "n1-standard-2"
}
```

</details>

## Create Autofill

When the template doesn't specify default values, Coder may still autofill
parameters.

You need to enable `auto-fill-parameters` first:

```shell
coder server --experiments=auto-fill-parameters
```

Or set the [environment variable](../../setup/index.md), `CODER_EXPERIMENTS=auto-fill-parameters`
With the feature enabled:

1. Coder will look for URL query parameters with form `param.<name>=<value>`.
   This feature enables platform teams to create pre-filled template creation
   links.
2. Coder will populate recently used parameter key-value pairs for the user.
   This feature helps reduce repetition when filling common parameters such as
   `dotfiles_url` or `region`.

## Dynamic Parameters

Dynamic Parameters enhances Coder's existing parameter system with real-time validation,
conditional parameter behavior, and richer input types.
This feature allows template authors to create more interactive and responsive workspace creation experiences.

### Enable Dynamic Parameters

<div class="tabs">

To use Dynamic Parameters, enable the experiment flag or set the environment variable:

#### Flag

```shell
coder server --experiments=dynamic-parameters
```

#### Env Variable

```shell
CODER_EXPERIMENTS=dynamic-parameters
```

</div>

Once enabled, users can toggle between the experimental and classic interfaces during
workspace creation.

### Features and Capabilities

Dynamic Parameters introduces three primary enhancements to the standard parameter system:

- **Conditional Parameters**

  - Parameters can respond to changes in other parameters
  - Show or hide parameters based on other selections
  - Modify validation rules conditionally
  - Create branching paths in workspace creation forms

- **Dynamic Data Fetching**

  - Fetch data at workspace creation time
  - Access Coder user attributes (groups, username, auth)
  - Selectively expose options based on user properties
  - Connect to external resources

- **Rich Parameter Entry**

  - Searchable dropdown lists for easier selection
  - Multi-select options for choosing multiple items
  - Secret text inputs for sensitive information
  - Key-value pair inputs for complex data
  - Button parameters for toggling sections

### Available Form Types

Dynamic Parameters supports a variety of form types to create rich, interactive user experiences:

<details><summary>**dropdown**: A searchable select menu for choosing a single option from a list</summary>

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

<details><summary>**radio**: Radio buttons for selecting a single option with high visibility</summary>

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

<details><summary>**multi-select**: Checkboxes for selecting multiple options from a list</summary>

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

<details><summary>**checkbox**: A single checkbox for boolean values</summary>

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

<details><summary>**switch**: A toggle switch for boolean values</summary>

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

<details><summary>**slider**: A slider for selecting numeric values within a range</summary>

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

<details><summary>**input**: A standard text input field</summary>

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

<details><summary>**textarea**: A multi-line text input field for longer content</summary>

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

<details><summary>**password**: A text input that masks sensitive information</summary>

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

<details><summary>**key-value**: Input for entering key-value pairs</summary>

```tf
data "coder_parameter" "environment_vars" {
  name         = "environment_vars"
  display_name = "Environment Variables"
  type         = "string"
  form_type    = "key-value"
  default      = jsonencode({"NODE_ENV": "development"})
}
```

</details>

### Examples

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
}

data "coder_parameter" "advanced_setting" {
  name         = "advanced_setting"
  display_name = "Advanced Setting"
  description  = "An advanced configuration option"
  type         = "string"
  default      = "default_value"
  mutable      = true

  # This parameter is only visible when show_advanced is true
  condition {
    field = data.coder_parameter.show_advanced.name
    value = "true"
  }
}
```

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

### API Details

Dynamic Parameters uses WebSocket communication to provide real-time updates.

The WebSocket endpoint is:

```api
GET /users/{user}/templateversions/{templateversion}/parameters
```

The WebSocket connection sends parameter changes as they occur and receives updated parameter state and validation errors in response.
