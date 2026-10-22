mock_provider "kubernetes" {}

variables {
  policy_id    = "test-lifecycle"
  namespace    = "coder"
  template_ids = ["template-a", "template-b"]
}

run "attach_with_defaults" {
  command = apply
  assert {
    condition     = length(kubernetes_manifest.policy) == 1 && length(output.manifest.spec.ingress) == 1 && length(output.manifest.spec.egress) == 1
    error_message = "P2P must be enabled by default and attached templates must create one policy."
  }
}

run "detach_all" {
  command = apply
  variables { template_ids = [] }
  assert {
    condition     = length(kubernetes_manifest.policy) == 0 && output.manifest == null
    error_message = "Detaching all templates must destroy the prior mocked policy."
  }
}

run "reattach" {
  command = plan
  variables { template_ids = ["template-c"] }
  assert {
    condition     = length(kubernetes_manifest.policy) == 1 && output.manifest.spec.endpointSelector.matchExpressions[0].values == tolist(["template-c"])
    error_message = "Reattachment must select only the newly attached template."
  }
}
