mock_provider "coder" {
  mock_data "coder_workspace" {
    defaults = {
      id          = "11111111-1111-4111-8111-111111111111"
      template_id = "ee88d119-881f-459f-91ac-ecf9c7347c52"
      start_count = 1
    }
  }
  mock_data "coder_template_outputs" {
    defaults = {
      outputs = { namespace = "snapshot-namespace" }
    }
  }
}

mock_provider "kubernetes" {}

run "namespace_snapshot" {
  command = plan

  assert {
    condition     = kubernetes_pod_v1.workspace[0].metadata[0].namespace == "snapshot-namespace" && kubernetes_persistent_volume_claim_v1.workspace.metadata[0].namespace == "snapshot-namespace"
    error_message = "Both pod and PVC must consume the supplied namespace, not a workspaces fallback."
  }
  assert {
    condition     = kubernetes_pod_v1.workspace[0].metadata[0].labels["coder.template_id"] == "ee88d119-881f-459f-91ac-ecf9c7347c52" && kubernetes_persistent_volume_claim_v1.workspace.metadata[0].labels["coder.template_id"] == "ee88d119-881f-459f-91ac-ecf9c7347c52"
    error_message = "Workspace labels must match the template identity used by the policy."
  }
}
