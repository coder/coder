# Workspace Tags

Template administrators can use static template tags to restrict workspace
provisioning to specific provisioner groups. However, this method has limited
flexibility as it prevents workspace users from creating workspaces on nodes of
their choice.

By using `coder_workspace_tags` and `coder_parameter`s, template administrators
can enable dynamic tag selection and modify static template tags.

## Dynamic tag selection

Here is a sample `coder_workspace_tags` data resource with a few workspace tags
specified:

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
- `os` - supported by the string-type `coder_parameter` to select OS
  runtime,`os_selector`
- `project_id` - a formatted string supported by the string-type
  `coder_parameter`, `project_name`
- `cache` - an HCL condition involving boolean-type `coder_parameter`,
  `feature_cache_enabled`

Review the
[full template example](https://github.com/coder/coder/tree/main/examples/workspace-tags)
using `coder_workspace_tags` and `coder_parameter`s.

## Constraints

### Tagged provisioners

With incorrectly selected workspace tags, it is possible to choose a tag
configuration that is not observed by any provisioner, causing the provisioner
job to get stuck in the queue indefinitely.

Before releasing the template version with configurable workspace tags, ensure
that every tag set is associated with at least one healthy provisioner.

### Parameters types

Provisioners require job tags to be defined in plain string format. When a
workspace tag refers to a `coder_parameter` without involving the string
formatter, for example, (`"os" = data.coder_parameter.os_selector.value`), the
Coder provisioner server can transform only the following parameter types to
strings: _string_, _number_, and _bool_.

### Mutability

A mutable `coder_parameter` can be dangerous for a workspace tag as it allows
the workspace owner to change a provisioner group (due to different tags). In
most cases, `coder_parameter`s backing `coder_workspace_tags` should be marked
as immutable and set only once, during workspace creation.

### HCL syntax

When importing the template version with `coder_workspace_tags`, the Coder
provisioner server extracts raw partial queries for each workspace tag and
stores them in the database. During workspace build time, the Coder server uses
the [Hashicorp HCL library](https://github.com/hashicorp/hcl) to evaluate these
raw queries on-the-fly without processing the entire Terraform template. This
evaluation is simpler but also limited in terms of available functions,
variables, and references to other resources.

**Supported syntax**

- Static string: `foobar_tag = "foobaz"`
- Formatted string: `foobar_tag = "foobaz ${data.coder_parameter.foobaz.value}"`
- Reference to `coder_parameter`:
  `foobar_tag = data.coder_parameter.foobar.value`
- Boolean logic: `production_tag = !data.coder_parameter.staging_env.value`
- Condition:
  `cache = data.coder_parameter.feature_cache_enabled.value == "true" ? "with-cache" : "no-cache"`
