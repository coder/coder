locals {
  model_catalog = jsondecode(file("${path.module}/models.json"))
  model_defaults = {
    claude   = "haiku"
    codex    = "gpt-6.1-sol"
    opencode = "anthropic/claude-haiku-5-5"
  }
  model_options = {
    claude = merge(local.model_catalog.claude_aliases, local.model_catalog.anthropic)
    codex  = local.model_catalog.openai
    opencode = merge(
      { for id, name in local.model_catalog.anthropic : "anthropic/${id}" => name },
      { for id, name in local.model_catalog.openai : "openai/${id}" => name },
    )
  }
}
