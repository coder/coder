---
title: Identity-aware parameters
---

Identity-aware parameters change based on the roles and groups of the user filling in the workspace creation form.
Use them to set governance policy directly in the form, rather than creating multiple templates to manage RBAC.

User identity is referenced in Terraform by reading the [`coder_workspace_owner`](https://registry.terraform.io/providers/coder/coder/latest/docs/data-sources/workspace_owner) data source.
Coder fills in this data source with the current user's identity each time the form renders, unlike other `data` sources, which Coder reads once at template import.
Refer to [How Coder evaluates parameters](./index.md#how-coder-evaluates-parameters).

To change parameters based on the user's other selections instead, refer to [Conditional parameters](./conditional-parameters.md).

## Role-aware options

Template administrators often want to expose certain experimental or unstable options only to those with elevated roles.
You can now do this by setting `count` based on a user's group or role, referencing the [`coder_workspace_owner`](https://registry.terraform.io/providers/coder/coder/latest/docs/data-sources/workspace_owner) data source.

[Try out admin-only options in the Playground](https://playground.coder.app/parameters/5Gn9W3hYs7).

```tf

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

## Group-aware regions

You can expose regions depending on which group a user belongs to.
This way developers can't accidentally induce low-latency with world-spanning connections.

[Try user-aware regions in the parameter playground](https://playground.coder.app/parameters/tBD-mbZRGm)

```tf

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

## Groups as namespaces

A slightly unorthodox way to leverage this is by filling the selections of a parameter from the user's groups.
Some users associate groups with namespaces, such as Kubernetes, then allow users to target that namespace with a parameter.

[Try groups as options in the Parameter Playground](https://playground.coder.app/parameters/lKbU53nYjl).

```tf
locals {
  groups = data.coder_workspace_owner.me.groups
}

data "coder_workspace_owner" "me" {}

data "coder_parameter" "your_groups" {
  type         = "string"
  name         = "your_groups"
  display_name = "Your Coder Groups"
  description  = "Select your namespace..."
  default      = "target-${local.groups[0]}"
  mutable      = true
  form_type = "dropdown"

  dynamic "option" {
    # options populated directly from groups
    for_each = local.groups
    content {
      name  = option.value
      # Native terraform be used to decorate output
      value = "target-${option.value}"
    }
  }
}
```
