terraform {
  required_providers {
    coder = {
      source  = "coder/coder"
      version = ">=2.0.0"
    }
  }
}

resource "coder_agent" "root" {
  os   = "linux"
  arch = "amd64"
}

resource "coder_devcontainer" "root" {
  agent_id         = coder_agent.root.id
  workspace_folder = "/workspace/root"
}

locals {
  root_agent_id        = coder_agent.root.id
  root_parent_agent_id = coder_devcontainer.root.agent_id
}

resource "coder_script" "local_agent" {
  agent_id     = local.root_agent_id
  display_name = "Local agent"
  script       = "echo local-agent"
  run_on_start = true
}

resource "coder_script" "local_parent" {
  agent_id     = local.root_parent_agent_id
  display_name = "Local parent"
  script       = "echo local-parent"
  run_on_start = true
}

module "runtime" {
  for_each = toset(["api", "worker"])
  source   = "./runtime"

  name = each.key
}

locals {
  module_agent_ids = {
    for name, runtime in module.runtime : name => runtime.agent_id
  }
  module_parent_agent_ids = {
    for name, runtime in module.runtime : name => runtime.parent_agent_id
  }
}

resource "coder_script" "module_agent" {
  for_each = local.module_agent_ids

  agent_id     = each.value
  display_name = "Module agent ${each.key}"
  script       = "echo module-agent-${each.key}"
  run_on_start = true
}

resource "coder_script" "module_parent" {
  for_each = local.module_parent_agent_ids

  agent_id     = each.value
  display_name = "Module parent ${each.key}"
  script       = "echo module-parent-${each.key}"
  run_on_start = true
}

resource "coder_agent" "service" {
  for_each = toset(["api", "worker"])

  os   = "linux"
  arch = "amd64"
}

module "feature" {
  for_each = coder_agent.service
  source   = "./feature"

  agent_id = each.value.id
  name     = each.key
}
