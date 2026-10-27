mock_provider "coder" {
  mock_data "coder_template_outputs" {
    defaults = {
      revision_id = "11111111-1111-4111-8111-111111111111"
      outputs = {
        namespace = "snapshot-namespace"
        greeting  = "Hello from a typed snapshot"
        structured = {
          enabled = true
          ports   = [443, 3100]
          mixed   = ["https", 443, true, null]
          nested  = { optional = null }
        }
      }
    }
  }
}

run "typed_snapshot" {
  command = plan

  assert {
    condition     = terraform_data.workspace_settings.input.namespace == "snapshot-namespace" && terraform_data.workspace_settings.input.greeting == "Hello from a typed snapshot"
    error_message = "Per-workspace resources must consume shared outputs directly."
  }
  assert {
    condition     = terraform_data.workspace_settings.input.structured.enabled == true && terraform_data.workspace_settings.input.structured.ports == [443, 3100] && terraform_data.workspace_settings.input.structured.mixed == ["https", 443, true, null] && terraform_data.workspace_settings.input.structured.nested.optional == null
    error_message = "Direct outputs must preserve nested values without string coercion."
  }
  assert {
    condition     = terraform_data.workspace_settings.input.template_version == "11111111-1111-4111-8111-111111111111"
    error_message = "The workspace must retain its output snapshot revision."
  }
}
