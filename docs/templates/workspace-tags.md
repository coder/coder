# Workspace Tags

Template admins can use static template tags to restrict workspace provisioning
to specific provisioner groups. This method has limitated flexibility as it
prevents workspace users from creating workspaces on workspace nodes of their
choice. Using `coder_workspace_tags` and `coder_parameter`s template admins can
enable dynamic tag selection.

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

## Limitations

### Parameters types

Provisioners require job tags to be defined in the plain string format. When a
workspace tag refers to a `coder_parameter` without involving the string
formatter, for example (`"os" = data.coder_parameter.os_selector.value`), Coder
provisioner server can transform to strings only following parameter types:
`string`, `number`, and `bool`.

#### Mutability

TODO

### HCL syntax

TODO
