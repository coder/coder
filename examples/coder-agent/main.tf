terraform {
  required_providers {
    coder = {
      version = "0.2"
      source  = "coder.com/internal/coder"
    }
  }
}

provider "coder" {
  // This would be passed as an ENV variable when running terraform (from provisionerd)
  coder_host_url = "http://localhost:3000"

  // This would probably never be actually used... but was testing passing through this to data sources and resources
  coder_agent_additional_args = "--verbose"

  coder_agent_environment_variable {
    name                 = "test"
    environment_variable = "TEST_ENV"
    value                = "Simple string"
  }

  coder_agent_environment_variable {
    name                 = "test2"
    environment_variable = "TEST_ENV2"
    value                = "Simple string"
  }
}

data "coder_agent" "agent" {}

output "coder_url" {
  value = data.coder_agent.agent.linux
}

output "script_path" {
  value = data.coder_agent.agent.linux
}
