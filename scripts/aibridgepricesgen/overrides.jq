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
