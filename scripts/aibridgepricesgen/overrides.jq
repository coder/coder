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

# gpt-6-astra: released 2026-09-03 and not listed on models.dev yet. Inject it
# from OpenAI's published model page and pricing table so the price book and
# the known-models catalog can carry it; drop this block once upstream lists it.
# Ref: https://developers.openai.com/api/docs/models/gpt-6-astra
# Ref: https://developers.openai.com/api/docs/pricing
| if (.openai.models | has("gpt-6-astra")) then
    error("overrides.jq: gpt-6-astra now present upstream; drop the injection")
  else
    .openai.models."gpt-6-astra" = {
      id: "gpt-6-astra",
      name: "GPT-6 Astra",
      attachment: true,
      reasoning: true,
      reasoning_options: [{type: "effort", values: ["low", "medium", "high", "xhigh", "max"]}],
      tool_call: true,
      structured_output: true,
      temperature: false,
      knowledge: "2026-04-30",
      release_date: "2026-09-03",
      last_updated: "2026-09-03",
      modalities: {input: ["text", "image"], output: ["text"]},
      open_weights: false,
      limit: {context: 1050000, input: 922000, output: 128000},
      cost: {
        input: 10,
        output: 50,
        cache_read: 1,
        cache_write: 12.5,
        tiers: [{input: 20, output: 75, cache_read: 2, cache_write: 25, tier: {type: "context", size: 272000}}]
      }
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
