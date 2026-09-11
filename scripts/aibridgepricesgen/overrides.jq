# Patches applied to the raw models.dev api.json before aibridgepricesgen
# consumes it. The Makefile pipes the fetched payload through this filter
# (jq -f scripts/aibridgepricesgen/overrides.jq) and both generated outputs
# (prices.json and knownModelsGenerated.json) read the patched snapshot.
#
# Every patch guards its assumption about upstream, so a stale override
# fails the pipeline loudly instead of silently patching nothing.

# claude-sonnet-4-5: models.dev advertises a 1M-token context window, which
# is incorrect. Anthropic retired the 1M context window beta on May 1st,
# 2026. Ref: https://platform.claude.com/docs/en/about-claude/models/overview
if .anthropic.models | has("claude-sonnet-4-5") then
  .anthropic.models."claude-sonnet-4-5".limit.context = 200000
else
  error("overrides.jq: claude-sonnet-4-5 gone from upstream; drop or update its context pin")
end

# claude-mythos-5: not listed on models.dev. Anthropic documents it as sharing
# claude-fable-5's specs and pricing, so inject it as a copy with its own
# id and display name.
# Ref: https://platform.claude.com/docs/en/about-claude/pricing#model-pricing
| if (.anthropic.models | has("claude-fable-5") | not) then
    error("overrides.jq: claude-fable-5 gone from upstream; the claude-mythos-5 copy has no source")
  elif (.anthropic.models | has("claude-mythos-5")) then
    error("overrides.jq: claude-mythos-5 now present upstream; drop the injection")
  else
    .anthropic.models."claude-mythos-5" = (
      .anthropic.models."claude-fable-5"
      | .id = "claude-mythos-5"
      | .name = "Claude Mythos 5"
    )
  end

# gpt-daybreak-blue-latest is an alias for gpt-5.6-sol. Copy its pricing
# until models.dev includes the alias. Recheck the target when OpenAI updates it.
# Ref: https://developers.openai.com/api/docs/pricing#cyber-models
| if (.openai.models | has("gpt-daybreak-blue-latest")) then
    error("overrides.jq: gpt-daybreak-blue-latest now present upstream; drop the injection")
  elif (.openai.models."gpt-5.6-sol".cost | (.input | type) != "number" or (.output | type) != "number") then
    error("overrides.jq: gpt-5.6-sol pricing missing upstream; update the gpt-daybreak-blue-latest source")
  else
    .openai.models."gpt-daybreak-blue-latest" = (
      .openai.models."gpt-5.6-sol"
      | .id = "gpt-daybreak-blue-latest"
      | .name = "GPT Daybreak Blue Latest"
    )
  end

# gpt-daybreak-red-latest is an alias for gpt-5.6-cyber. Neither is listed
# on models.dev, so use OpenAI's USD-per-million-token prices directly.
# Recheck the target and rates when OpenAI updates the alias.
# Ref: https://developers.openai.com/api/docs/pricing#cyber-models
| if (.openai.models | has("gpt-daybreak-red-latest")) then
    error("overrides.jq: gpt-daybreak-red-latest now present upstream; drop the injection")
  elif (.openai.models | has("gpt-5.6-cyber")) then
    error("overrides.jq: gpt-5.6-cyber now present upstream; copy its pricing for gpt-daybreak-red-latest")
  else
    .openai.models."gpt-daybreak-red-latest" = {
      id: "gpt-daybreak-red-latest",
      name: "GPT Daybreak Red Latest",
      cost: {input: 12.5, output: 75, cache_read: 1.25, cache_write: 15.625}
    }
  end

# Mapping of provider names on models.dev to our own names
# Ref. table definition for ai_provider_type
# amazon-bedrock -> bedrock
# github-copilot -> copilot
| if (has("amazon-bedrock") | not) then
   error("overrides.jq: amazon-bedrock not present upstream; drop or update the rename")
else
  .bedrock = ."amazon-bedrock" | del(."amazon-bedrock")
end
| if (has("github-copilot") | not) then
   error("overrides.jq: github-copilot not present upstream; drop or update the rename")
else
  .copilot = ."github-copilot" | del(."github-copilot")
end
