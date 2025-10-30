terraform {
  required_providers {
    coder = {
      source  = "coder/coder"
      version = "2.5.3"
    }
  }
}

locals {
  num_presets = {{.NumPresets}}
}

resource "null_resource" "workspace" {}


data "coder_workspace_preset" "presets" {
  count = local.num_presets
  name     = "preset-${count.index + 1}"
  prebuilds {
    instances = {{.NumPresetPrebuilds}}
  }
}
