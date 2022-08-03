terraform {
  required_providers {
    coder = {
      source = "coder/coder"
    }
  }
}

# This example does not provision any resources. Use this
# template as a starting point for writing custom templates
# using any Terraform resource/provider.
#
# See: https://coder.com/docs/coder-oss/latest/templates

data "coder_workspace" "me" {
}

resource "coder_agent" "main" {
  os   = "linux"
  arch = "amd64"
  auth = "token"
}

resource "null_resource" "fake-compute" {
  # When a workspace is stopped, this resource is destroyed.
  count = data.coder_workspace.me.transition == "start" ? 1 : 0

  provisioner "local-exec" {
    command = "echo 🔊 ${data.coder_workspace.me.owner} has started a workspace named ${data.coder_workspace.me.name}"
  }

  # Run the Coder agent init script on resources
  # to access web apps and SSH:
  #
  # export CODER_AGENT_TOKEN=${coder_agent.main.token}
  # ${coder_agent.main.init_script}
}

resource "null_resource" "fake-disk" {
  # This resource will remain even when workspaces are restarted.
  count = 1
}

resource "coder_app" "fake-app" {
  # Access :8080 in the workspace from the Coder dashboard.
  name     = "VS Code"
  icon     = "/icon/code.svg"
  agent_id = "fake-compute"
  url      = "http://localhost:8080"
}
