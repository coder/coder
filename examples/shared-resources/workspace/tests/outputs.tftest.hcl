mock_provider "coder" {
  mock_data "coder_foundation" {
    defaults = {
      id           = "22222222-2222-4222-8222-222222222222"
      name         = "example-foundation"
      template_ids = ["33333333-3333-4333-8333-333333333333"]
      snapshot_id  = "11111111-1111-4111-8111-111111111111"
      outputs = {
        namespace  = "snapshot-namespace"
        greeting   = "Hello from a typed snapshot"
        public_set = toset(["http", "https"])
        structured = {
          enabled = true
          ports   = [443, 3100]
          mixed   = ["https", 443, true, null]
          nested  = { optional = null }
        }
      }
      # Mock values check consumption, not the real provider's sensitivity marks.
      sensitive_outputs = {
        private_setting = "demo-only-not-a-real-secret"
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
    condition     = output.namespace == "snapshot-namespace"
    error_message = "The namespace output must expose the supplied Foundation value without a fallback."
  }
  assert {
    condition     = toset(keys(terraform_data.public_setting)) == toset(["http", "https"])
    error_message = "Public identity keys must work directly with for_each alongside sensitive roots."
  }
  assert {
    condition     = !contains(keys(data.coder_foundation.current.outputs), "private_setting") && !contains(keys(data.coder_foundation.current.sensitive_outputs), "namespace")
    error_message = "The snapshot fixture must keep public and sensitive roots in separate objects."
  }
  assert {
    condition     = terraform_data.workspace_settings.input.private_setting == "demo-only-not-a-real-secret"
    error_message = "Sensitive root values must remain available to workspace resources."
  }
  assert {
    condition     = terraform_data.workspace_settings.input.structured.enabled == true && terraform_data.workspace_settings.input.structured.ports == [443, 3100] && terraform_data.workspace_settings.input.structured.mixed == ["https", 443, true, null] && terraform_data.workspace_settings.input.structured.nested.optional == null
    error_message = "Direct outputs must preserve nested values without string coercion."
  }
  assert {
    condition     = terraform_data.workspace_settings.input.foundation_snapshot == "11111111-1111-4111-8111-111111111111"
    error_message = "The workspace must retain its output snapshot's producing run ID."
  }
  assert {
    condition     = terraform_data.workspace_settings.input.foundation_id == "22222222-2222-4222-8222-222222222222" && terraform_data.workspace_settings.input.snapshot_foundation == terraform_data.workspace_settings.input.foundation_id
    error_message = "Foundation metadata and output provenance must identify the Foundation, not its template or revision."
  }
  assert {
    condition     = terraform_data.workspace_settings.input.foundation_name == "example-foundation" && tolist(terraform_data.workspace_settings.input.foundation_templates) == tolist(["33333333-3333-4333-8333-333333333333"])
    error_message = "Foundation metadata must preserve its name and contextual template IDs."
  }
}
