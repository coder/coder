# The state of this template exercises how `terraform show -json` orders and
# addresses entries: data sources, counted and keyed resources and modules,
# nested modules, and unusual keys.
terraform {
  required_providers {
    coder = {
      source  = "coder/coder"
      version = "2.15.0"
    }
  }
}

variable "fail_provisioner" {
  type    = bool
  default = false
}

data "coder_workspace" "me" {}

data "coder_parameter" "region" {
  name    = "region"
  default = "us"
  option {
    name  = "US"
    value = "us"
  }
  option {
    name  = "EU"
    value = "eu"
  }
}

data "coder_parameter" "cpus" {
  name    = "cpus"
  type    = "number"
  default = 2
  validation {
    min = 1
    max = 8
  }
}

data "coder_external_auth" "github" {
  id       = "github"
  optional = true
}

resource "coder_agent" "main" {
  arch = "amd64"
  os   = "linux"
  env  = { REGION = data.coder_parameter.region.value }
  metadata {
    key          = "load"
    display_name = "Load"
    script       = "uptime"
    interval     = 10
  }
}

# terraform_data.input is dynamically typed, so the state file records its
# type next to its value.
resource "terraform_data" "host" {
  input = coder_agent.main.token
}

resource "coder_metadata" "host" {
  resource_id = terraform_data.host.id
  item {
    key   = "region"
    value = data.coder_parameter.region.value
  }
  item {
    key       = "password"
    value     = "hunter2"
    sensitive = true
  }
}

resource "coder_app" "apps" {
  for_each     = toset(["code", "web ui"])
  agent_id     = coder_agent.main.id
  slug         = replace(each.key, " ", "-")
  display_name = each.key
  url          = "http://localhost:8080"
  healthcheck {
    url       = "http://localhost:8080/healthz"
    interval  = 5
    threshold = 3
  }
}

# Integer keys sort numerically: [10] follows [9].
resource "coder_env" "counted" {
  count    = 11
  agent_id = coder_agent.main.id
  name     = "COUNTED_${count.index}"
  value    = tostring(count.index)
}

# String keys sort bytewise and are quoted in addresses the way Terraform
# quotes them, which escapes template introducers and non-printable runes.
resource "coder_env" "keys" {
  for_each = toset([
    "", "B", "a", "web ui", "quote\"d", "back\\slash", "a$${b}", "c%%{d}",
    "tab\there", "new\nline", "cr\rlf\n", "bell\u0007", "del\u007f",
    "nbsp\u00a0", "ünïcödé", "🚀",
  ])
  agent_id = coder_agent.main.id
  name     = "KEY_${md5(each.key)}"
  value    = each.key
}

# When fail_provisioner is set, replacing this agent leaves a tainted new
# object and keeps the old one as a deposed object. Nothing hosts the agent:
# its current and deposed graph nodes share a label, and ConvertState picks
# either one.
resource "coder_agent" "replaced" {
  arch = "amd64"
  os   = "linux"
  lifecycle {
    create_before_destroy = true
  }
  provisioner "local-exec" {
    command = var.fail_provisioner ? "exit 1" : "exit 0"
  }
}

# module.child holds no resources itself, and its key contains ".module.".
module "child" {
  for_each = toset(["x.module.y"])
  source   = "./child"
  agent_id = coder_agent.main.id
}

# Module instances sort by address: [10] precedes [2].
module "counted" {
  count    = 11
  source   = "./child/grandchild"
  agent_id = coder_agent.main.id
}

# module.wrap sorts before module.wrap-sibling, but holds no resources itself,
# and the state file lists module.wrap.module.grandchild after
# module.wrap-sibling.
module "wrap" {
  source   = "./child"
  agent_id = coder_agent.main.id
}

module "wrap-sibling" {
  source   = "./child/grandchild"
  agent_id = coder_agent.main.id
}
