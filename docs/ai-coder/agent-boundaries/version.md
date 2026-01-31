# Version Requirements

## Recommended Versions

It's recommended to use **Coder v2.30.0 or newer** and **Claude Code module v4.7.0 or newer**.

### Coder v2.30.0+

Since Coder v2.30.0, Agent Boundaries is embedded inside the Coder binary, and you don't need to install it separately. The `coder boundary` subcommand is available directly from the Coder CLI.

### Claude Code Module v4.7.0+

Since Claude Code module v4.7.0, the embedded `coder boundary` subcommand is used by default. This means you don't need to set `boundary_version`; the boundary version is tied to your Coder version.

## Compatibility with Older Versions

### Using Coder Before v2.30.0 with Claude Code Module v4.7.0+

If you're using Coder before v2.30.0 with Claude Code module v4.7.0 or newer, the `coder boundary` subcommand isn't available in your Coder installation. In this case, you need to:

1. Set `use_boundary_directly = true` in your Terraform module configuration
2. Explicitly set `boundary_version` to specify which Agent Boundaries version to install

Example configuration:

```tf
module "claude-code" {
  source              = "dev.registry.coder.com/coder/claude-code/coder"
  version             = "4.7.0"
  enable_boundary     = true
  use_boundary_directly = true
  boundary_version    = "0.6.0"
}
```

### Using Claude Code Module Before v4.7.0

If you're using Claude Code module before v4.7.0, the module expects to use Agent Boundaries directly. You need to explicitly set `boundary_version` in your Terraform configuration:

```tf
module "claude-code" {
  source              = "dev.registry.coder.com/coder/claude-code/coder"
  version             = "4.6.0"
  enable_boundary     = true
  boundary_version    = "0.6.0"
}
```

## Summary

| Coder Version | Claude Code Module Version | Configuration Required                                |
|---------------|----------------------------|-------------------------------------------------------|
| v2.30.0+      | v4.7.0+                    | No additional configuration needed                    |
| < v2.30.0     | v4.7.0+                    | `use_boundary_directly = true` and `boundary_version` |
| Any           | < v4.7.0                   | `boundary_version`                                    |
