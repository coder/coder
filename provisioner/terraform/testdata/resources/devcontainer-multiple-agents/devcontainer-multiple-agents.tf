terraform {
  required_providers {
    coder = {
      source  = "coder/coder"
      version = ">=2.0.0"
    }
  }
}

# Two agents, but the devcontainer only depends on one.
# This tests the continue path when iterating agents for devcontainer association.
resource "coder_agent" "main" {
  os   = "linux"
  arch = "amd64"
}

resource "coder_agent" "secondary" {
  os   = "linux"
  arch = "amd64"
}

# This devcontainer only depends on the main agent.
resource "coder_devcontainer" "dev" {
  agent_id         = coder_agent.main.id
  workspace_folder = "/workspace"
}

# A second devcontainer that also depends on main agent.
# This allows us to test the dependsOnDevcontainer returning false
# when checking if an app belongs to this devcontainer vs dev.
resource "coder_devcontainer" "other" {
  agent_id         = coder_agent.main.id
  workspace_folder = "/other"
}

# This app depends on "dev" devcontainer, not "other".
# When iterating devcontainers, dependsOnDevcontainer should return
# false for "other" and true for "dev".
resource "coder_app" "devcontainer-app" {
  agent_id = coder_devcontainer.dev.subagent_id
  slug     = "devcontainer-app"
}

resource "null_resource" "dev" {
  depends_on = [
    coder_agent.main
  ]
}

resource "null_resource" "secondary" {
  depends_on = [
    coder_agent.secondary
  ]
}
