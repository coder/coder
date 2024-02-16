# UI Resource Ordering

In Coder templates, managing the order of UI elements is crucial for a seamless
user experience. This page outlines how resources can be aligned using the
`order` Terraform property or inherit the natural order from the file.

The resource with the lower `order` is presented before the one with greater
value. A missing `order` property defaults to 0. If two resources have the same
`order` property, the resources will be ordered by property `name` (or `key`).

## Using `order` property

### Coder parameters

The `order` property of `coder_parameter` resource allows specifying the order
of parameters in UI forms. In the below example, `project_id` will appear
_before_ `account_id`:

```hcl
data "coder_parameter" "project_id" {
  name         = "project_id"
  display_name = "Project ID"
  description  = "Specify cloud provider project ID."
  order = 2
}

data "coder_parameter" "account_id" {
  name         = "account_id"
  display_name = "Account ID"
  description  = "Specify cloud provider account ID."
  order = 1
}
```

### Agents

Agent resources within the UI left pane are sorted based on the `order`
property, followed by `name`, ensuring a consistent and intuitive arrangement.

```hcl
resource "coder_agent" "primary" {
  ...

  order = 1
}

resource "coder_agent" "secondary" {
  ...

  order = 2
}
```

The agent with the lowest order is presented at the top in the workspace view.

### Agent metadata

The `coder_agent` exposes metadata to present operational metrics in the UI.
Metrics defined with Terraform `metadata` blocks can be ordered using additional
`order` property; otherwise, they are sorted by `key`.

```hcl
resource "coder_agent" "main" {
  ...

  metadata {
    display_name = "CPU Usage"
    key          = "cpu_usage"
    script       = "coder stat cpu"
    interval     = 10
    timeout      = 1
    order        = 1
  }
  metadata {
    display_name = "CPU Usage (Host)"
    key          = "cpu_usage_host"
    script       = "coder stat cpu --host"
    interval     = 10
    timeout      = 1
    order        = 2
  }
  metadata {
    display_name = "RAM Usage"
    key          = "ram_usage"
    script       = "coder stat mem"
    interval     = 10
    timeout      = 1
    order        = 1
  }
  metadata {
    display_name = "RAM Usage (Host)"
    key          = "ram_usage_host"
    script       = "coder stat mem --host"
    interval     = 10
    timeout      = 1
    order        = 2
  }
}
```

### Applications

Similarly to Coder agents, `coder_app` resources incorporate the `order`
property to organize button apps in the app bar within a `coder_agent` in the
workspace view.

Only template defined applications can be arranged. _VS Code_ or _Terminal_
buttons are static.

```hcl
resource "coder_app" "code-server" {
  agent_id     = coder_agent.main.id
  slug         = "code-server"
  display_name = "code-server"
  ...

  order = 2
}

resource "coder_app" "filebrowser" {
  agent_id     = coder_agent.main.id
  display_name = "File Browser"
  slug         = "filebrowser"
  ...

  order = 1
}
```

## Inherit order from file

### Coder parameter options

The options for Coder parameters maintain the same order as in the file
structure. This simplifies management and ensures consistency between
configuration files and UI presentation.

```hcl
data "coder_parameter" "database_region" {
  name         = "database_region"
  display_name = "Database Region"

  icon        = "/icon/database.svg"
  description = "These are options."
  mutable     = true
  default     = "us-east1-a"

  // The order of options is stable and inherited from .tf file.
  option {
    name        = "US Central"
    description = "Select for central!"
    value       = "us-central1-a"
  }
  option {
    name        = "US East"
    description = "Select for east!"
    value       = "us-east1-a"
  }
  ...
}
```

### Coder metadata items

In cases where multiple item properties exist, the order is inherited from the
file, facilitating seamless integration between a Coder template and UI
presentation.

```hcl
resource "coder_metadata" "attached_volumes" {
  resource_id = docker_image.main.id

  // Items will be presented in the UI in the following order.
  item {
    key   = "disk-a"
    value = "60 GiB"
  }
  item {
    key   = "disk-b"
    value = "128 GiB"
  }
}
```
