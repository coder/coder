# Resource monitoring

Use the
[`resources_monitoring`](https://registry.terraform.io/providers/coder/coder/latest/docs/resources/agent#resources_monitoring-1)
block on the
[`coder_agent`](https://registry.terraform.io/providers/coder/coder/latest/docs/resources/agent)
resource in our Terraform provider to monitor out of memory (OOM) and out of
disk (OOD) errors and alert users when they overutilize memory and disk.

This can help prevent agent disconnects due to OOM/OOD issues.

You can specify one or more volumes to monitor for OOD alerts.
OOM alerts are reported per-agent.

## Example

Add the following example to the template's `main.tf`.
Change the `90`, `80`, and `95` to a threshold that's more appropriate for your
deployment:

```hcl
resource "coder_agent" "main" {
  arch = data.coder_provisioner.dev.arch
  os   = data.coder_provisioner.dev.os
  resources_monitoring {
    memory {
      enabled   = true
      threshold = 90
    }
    volume {
      path      = "/volume1"
      enabled   = true
      threshold = 80
    }
    volume {
      path      = "/volume2"
      enabled   = true
      threshold = 95
    }
  }
}
```
