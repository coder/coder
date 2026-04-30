# Frontend Known Model catalog for onboarding defaults

We use a checked-in frontend **Known Model** catalog for OpenAI and Anthropic
onboarding defaults instead of runtime provider discovery or backend-seeded
Model Configs. This keeps the add-model flow deterministic, testable, and
frontend-only, while preserving saved Model Configs as the source of truth
after submission. The catalog is advisory: it uses latest non-date aliases as
canonical onboarding identifiers, maps only objective metadata supported by the
existing form, and intentionally excludes tiered pricing and provider-specific
reasoning defaults until those concepts are represented by backend-supported
product models.
