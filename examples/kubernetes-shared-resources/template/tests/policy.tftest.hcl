mock_provider "coder" {
  mock_data "coder_foundation" {
    defaults = {
      id                = "22222222-2222-4222-8222-222222222222"
      name              = "serpent-foundation"
      template_ids      = ["ee88d119-881f-459f-91ac-ecf9c7347c52"]
      outputs           = {}
      sensitive_outputs = {}
    }
  }
  mock_data "coder_template" {
    defaults = {
      id   = "ee88d119-881f-459f-91ac-ecf9c7347c52"
      name = "serpent-dev"
    }
  }
}

mock_provider "kubernetes" {}

run "template_policy_module" {
  command = plan

  assert {
    condition     = output.foundation_id == "22222222-2222-4222-8222-222222222222" && output.foundation_id != output.template_id
    error_message = "Foundation self metadata must not replace the attached template identity."
  }
  assert {
    condition     = output.namespace == "workspaces" && module.coder_network_policy.manifest.metadata.namespace == output.namespace
    error_message = "The policy module must consume the owned namespace exported to workspaces."
  }
  assert {
    condition     = module.coder_network_policy.policy_name == "coder-policy-${output.template_id}" && module.coder_network_policy.manifest.spec.endpointSelector.matchExpressions[0].values == tolist([output.template_id])
    error_message = "The policy name and selector must use this template's identity."
  }
  assert {
    condition     = module.coder_network_policy.manifest.spec.enableDefaultDeny.ingress && module.coder_network_policy.manifest.spec.enableDefaultDeny.egress && length(module.coder_network_policy.manifest.spec.ingress) == 0 && length(module.coder_network_policy.manifest.spec.egress) == 3
    error_message = "The demo must disable direct P2P and allow only DNS, HTTPS, and the control plane."
  }
  assert {
    condition     = module.coder_network_policy.manifest.spec.egress[1].toPorts[0].serverNames == tolist(["github.com", "proxy.golang.org", "sum.golang.org"]) && length(module.coder_network_policy.manifest.spec.egress[0].toPorts[0].rules.dns) == 3
    error_message = "The module must receive only the three approved HTTPS/DNS names."
  }
  assert {
    condition     = module.coder_network_policy.manifest.spec.egress[2].toCIDR[0] == "172.20.0.3/32" && module.coder_network_policy.manifest.spec.egress[2].toPorts[0].ports[0].port == "3100"
    error_message = "The control-plane allowance must remain the exact local endpoint."
  }
}
