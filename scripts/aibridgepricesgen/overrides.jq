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

# claude-mythos-5-1: same situation as claude-mythos-5. Anthropic prices it
# identically to claude-fable-5-1 (including the reduced cache-read rate),
# so inject it as a copy with its own id and display name.
# Ref: https://platform.claude.com/docs/en/about-claude/pricing#model-pricing
| if (.anthropic.models | has("claude-fable-5-1") | not) then
    error("overrides.jq: claude-fable-5-1 gone from upstream; the claude-mythos-5-1 copy has no source")
  elif (.anthropic.models | has("claude-mythos-5-1")) then
    error("overrides.jq: claude-mythos-5-1 now present upstream; drop the injection")
  else
    .anthropic.models."claude-mythos-5-1" = (
      .anthropic.models."claude-fable-5-1"
      | .id = "claude-mythos-5-1"
      | .name = "Claude Mythos 5.1"
    )
  end

# anthropic.claude-sonnet-5-5: bedrock-runtime serves this model only through
# the global inference profile, which is all models.dev lists. bedrock-mantle
# takes the bare ID, and models.dev prices bare Claude IDs like their global
# profiles, so inject the bare ID as a copy of the global entry.
# Ref: https://docs.aws.amazon.com/bedrock/latest/userguide/model-card-anthropic-claude-sonnet-5-5.html
| if (."amazon-bedrock".models | has("global.anthropic.claude-sonnet-5-5") | not) then
    error("overrides.jq: global.anthropic.claude-sonnet-5-5 gone from upstream; the anthropic.claude-sonnet-5-5 copy has no source")
  elif (."amazon-bedrock".models | has("anthropic.claude-sonnet-5-5")) then
    error("overrides.jq: anthropic.claude-sonnet-5-5 now present upstream; drop the injection")
  else
    ."amazon-bedrock".models."anthropic.claude-sonnet-5-5" = (
      ."amazon-bedrock".models."global.anthropic.claude-sonnet-5-5"
      | .id = "anthropic.claude-sonnet-5-5"
      | .name = "Claude Sonnet 5.5"
    )
  end

# Copilot does not charge for background utility calls using GPT-4o mini,
# GPT-4o, or GPT-4.1, so record zero prices for these models.
# Ref: https://docs.github.com/en/copilot/concepts/models/utility-models#list-of-utility-models
#
# Keep GPT-5.4 nano's upstream rates: utility calls are unbilled, but other
# usage has published prices. The model ID alone cannot distinguish them.
# Ref: https://docs.github.com/en/copilot/reference/copilot-billing/models-and-pricing#openai
| if (has("github-copilot") | not) then
    error("overrides.jq: github-copilot missing upstream; update the utility model overrides")
  else
    reduce [
      {id: "gpt-4o-mini", name: "GPT-4o mini"},
      {id: "gpt-4o", name: "GPT-4o"},
      {id: "gpt-4.1", name: "GPT-4.1"}
    ][] as $model (.;
      if (."github-copilot".models | has($model.id)) then
        error("overrides.jq: github-copilot/\($model.id) now present upstream; review and drop the injection")
      else
        ."github-copilot".models[$model.id] = (
          $model | .cost = {input: 0, output: 0, cache_read: 0, cache_write: 0}
        )
      end
    )
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
