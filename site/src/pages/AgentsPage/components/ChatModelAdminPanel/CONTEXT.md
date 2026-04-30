# Chat Model Admin Panel

This is the admin-facing UI for configuring AI **Providers** and the **Model
Configs** that route through them. Code lives in
`site/src/pages/AgentsPage/components/ChatModelAdminPanel/`.

## Language

**Provider**:
A configured external AI service (Anthropic, OpenAI, Bedrock, etc.) holding
credentials and base URL. Backed by the `chat_providers` table.
_Avoid_: vendor, integration.

**Model Config**:
A persisted, admin-defined entry that pairs a **Model Identifier** with a
**Provider** plus operational metadata (context limit, compression threshold,
provider-specific options). One row in `chat_model_configs`.
_Avoid_: "model" on its own (ambiguous with Model Identifier).

**Model Identifier**:
The exact string sent to the provider API as the `model` parameter, e.g.
`gpt-5` or `claude-sonnet-4-5`. The Model Config form labels this field
"Model Identifier".
_Avoid_: model name, model ID alone (overloaded with database row id).

**Known Model**:
A curated entry in the frontend **Model Catalog** with prefill metadata
(display name, context limit, pricing, provider-specific defaults) for a
canonical Model Identifier. Maintained as a checked-in snapshot from
models.dev. Not persisted; never sent to the backend.
_Avoid_: preset, default model, recommended model.

**Model Catalog**:
The frontend-only, per-Provider list of **Known Model** entries that powers
discovery suggestions on the Model Identifier field and prefills the
Model Config form. It is checked in as curated TypeScript records with
models.dev source metadata, not as a full external snapshot. Not
authoritative; the backend remains the source of truth after save.
_Avoid_: model registry, catalogue, presets.

**Default application**:
The act of copying advisory metadata from a **Known Model** into a draft
**Model Config** form. It happens only in add mode, immediately on explicit
Known Model selection or on blur after an exact canonical Model Identifier
match, and fills only target fields whose values still match the form's
initial values.
_Avoid_: auto-creation, import, sync.

**Off-catalog Model Identifier**:
A Model Identifier entered by an admin that does not match any **Known Model**
for the selected **Provider**. It remains valid form input because the
**Model Catalog** is advisory, not authoritative.
_Avoid_: unsupported model, invalid model.

**Discovery suggestions**:
The autocomplete list of **Known Model** entries shown beneath the Model
Identifier input, filtered by selected **Provider**. Populated from the
**Model Catalog**.
_Avoid_: typeahead options, recommendations.

## Initial Model Catalog scope

The first catalog supports only native `openai` and `anthropic` providers.
The initial OpenAI entries are `gpt-5.5`, `gpt-5.5-pro`, `gpt-5.4`,
`gpt-5.4-mini`, `gpt-5.4-nano`, and `gpt-5.3-codex`. The initial Anthropic
entries are `claude-opus-4-7`, `claude-opus-4-6`, `claude-sonnet-4-6`,
`claude-haiku-4-5`, and `claude-sonnet-4-5`. Do not include GPT-4.x,
pre-5.3 GPT models, or Claude models older than 4.5 in the onboarding
catalog unless product intentionally expands the scope.

## Canonical Model Identifier rule

For onboarding **Known Models**, prefer the provider's non-date latest alias as
the canonical **Model Identifier** when one exists, such as `gpt-5.5` or
`claude-sonnet-4-6`. Date-pinned provider identifiers may be search aliases,
but selecting a Known Model writes the non-date canonical identifier into the
form. Admins who need date-pinned reproducibility can type an
**Off-catalog Model Identifier** manually.

## Compression threshold rule

The **Model Catalog** does not set compression threshold values in the first
pass. Compression threshold is Coder application policy, not provider model
metadata. If a better default is needed, change the form-level default for all
Model Configs rather than encoding it per Known Model.

## Model limit mapping

When deriving **Known Model** defaults from models.dev, map
`limit.context` to the Model Config `contextLimit` field. Map `limit.output`
to the selected provider's exact max-output-tokens field when one exists;
otherwise map it to the generic `config.maxOutputTokens` field. Never fill
both generic and provider-specific output-token fields for the same Known
Model. Ignore `limit.input` in the first pass unless the form schema already
exposes an exact matching field.

## Pricing mapping

When deriving **Known Model** defaults from models.dev, map only the base flat
pricing fields that the existing Model Config form can persist: `cost.input`,
`cost.output`, `cost.cache_read`, and `cost.cache_write`. Tiered pricing such
as `context_over_200k` is intentionally out of scope for the first pass
because Coder does not yet have a backend-supported tiered pricing model.
Editable large-context billing fields belong in a separate backend-owned
pricing feature, not in frontend-only onboarding defaults.

## Provider reasoning options

The **Model Catalog** does not set provider-specific reasoning or thinking
options in the first pass. Generic `reasoning` metadata from models.dev does
not map cleanly to OpenAI reasoning effort/summary or Anthropic thinking
controls, and there is no existing Coder product default to reuse. Future
reasoning defaults should be explicit provider-specific product decisions.

## Model Identifier field behavior

In the first pass, the autocomplete behavior applies only to add-mode forms
for providers with **Known Models**. Edit mode, duplicate mode, and providers
without catalog entries keep the existing free-text Model Identifier input
behavior.

## Default application feedback

After **Default application** changes at least one form field, show a small
inline note near the Model Identifier field, such as "Defaults applied from
GPT-5.5. Review and adjust before saving." Do not show this feedback for
off-catalog identifiers or Known Model selections that do not change any
fields.

## Default application result

The pure helper that performs **Default application** returns both the next
form values and the list of form paths it populated. UI feedback is driven by
whether at least one path was populated, not by ad-hoc before/after checks in
React components.

## Known Model search

Discovery suggestions search over a Known Model's canonical **Model
Identifier**, display name, and explicit aliases. Matching is case-insensitive
and normalizes spaces, hyphens, underscores, and dots before substring checks.
Aliases are objective name or identifier variants, such as marketing names,
punctuation variants, and date-pinned identifiers for the same model. Do not
use editorial intent tags such as `best`, `cheap`, `fast`, `coding`, or
`reasoning`, and do not implement typo-tolerant fuzzy matching in the first
pass.

## Existing Model Configs and suggestions

Discovery suggestions do not hide or mark Known Models that already have saved
Model Configs for the selected Provider. The data model permits multiple Model
Configs with the same Provider and Model Identifier, so suggestions are based
only on selected Provider and search text.

## Provider CTA scope

The first pass improves the add-model form itself and does not add a provider
success popover, provider-side call to action, or new deep-link behavior. If
admins still struggle to move from Provider setup to Model Config setup, that
is a separate onboarding feature.

## Relationships

- A **Provider** has zero or more **Model Configs** (1:N via `provider` FK).
- A **Provider** has zero or more **Known Models** in the **Model Catalog**.
- Picking a **Known Model** prefills a draft **Model Config** but does not
  create one until the admin submits the form.

## Example dialogue

> **PM:** "Can the admin pick GPT-5 from a list?"
> **Dev:** "Only if it's in the **Model Catalog**. Otherwise they type the
> **Model Identifier** by hand. Either way they end up with a saved
> **Model Config**, which is what the chat runtime actually uses."

## Flagged ambiguities

- "model" was used to mean the Model Identifier string, the Model Config
  database row, and the Known Model catalog entry. Resolved: these are three
  distinct concepts. Use "Model Identifier", "Model Config", and "Known
  Model" respectively.
