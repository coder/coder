# Workspace Tags

Template admins can use static template tags to restrict workspace provisioning
to specific provisioner groups. This method has limited flexibility as it
prevents workspace users from creating workspaces on workspace nodes of their
choice.

Using `coder_workspace_tags` and `coder_parameter`s template admins can enable
dynamic tag selection, and mutate static template tags.

## Dynamic tag selection

Here is a sample `coder_workspace_tags` data resource with a couple of workspace
tags specified:

```hcl
data "coder_workspace_tags" "custom_workspace_tags" {
  tags = {
    "zone"        = "developers"
    "os"          = data.coder_parameter.os_selector.value
    "project_id"  = "PROJECT_${data.coder_parameter.project_name.value}"
    "cache"       = data.coder_parameter.feature_cache_enabled.value == "true" ? "with-cache" : "no-cache"
  }
}
```

**Legend**

- `zone` - static tag value set to `developers`
- `os` - supported by string `coder_parameter` to select OS
  runtime,`os_selector`
- `project_id` - a formatted string supported by string `coder_parameter`,
  `project_name`
- `cache` - an HCL condition involving boolean `coder_parameter`,
  `feature_cache_enabled`

Review the
[full template example](https://github.com/coder/coder/tree/main/examples/workspace-tags)
using `coder_workspace_tags` and `coder_parameter`s.

## Contraints

### Tagged provisioners

With incorrectly selected workspace tags it is possible to pick a tag
configuration that is not observed by any provisioner, and make the provisioner
job stuck in the queue indefinitely.

Before releasing the template version with configurable workspace tags, make
sure that every tag set is related with at least one healthy provisioner.

### Parameters types

Provisioners require job tags to be defined in the plain string format. When a
workspace tag refers to a `coder_parameter` without involving the string
formatter, for example (`"os" = data.coder_parameter.os_selector.value`), Coder
provisioner server can transform to strings only following parameter types:
`string`, `number`, and `bool`.

### Mutability

A mutable `coder_parameter` might be dangerous for a workspace tag as it allows
the workspace owner to change a provisioner group (due to different tags). In
majority of use cases, `coder_parameter`s backing `coder_workspace_tags` should
be marked as _immutable_ and set only once, during the workspace creation.

### HCL syntax

While importing the template version with `coder_workspace_tags`, Coder
provisioner server extracts raw partial query for every workspace tag and stores
it in the database. During workspace build time, Coder server uses
[Hashicorp HCL library](github.com/hashicorp/hcl/v2) to evaluate these raw
queries _in-the-fly_ without processing the entire Terraform template. Such
evaluation is simpler, but also limited in terms of available functions,
variables, and references to other resources.

**Supported syntax**

- static string: `foobar_tag = "foobaz"`
- formatted string: `foobar_tag = "foobaz ${data.coder_parameter.foobaz.value}"`
- reference to `coder_parameter`:
  `foobar_tag = data.coder_parameter.foobar.value`
- boolean logic: `production_tag = !data.coder_parameter.staging_env.value`
- condition:
  `cache = data.coder_parameter.feature_cache_enabled.value == "true" ? "with-cache" : "no-cache"`
