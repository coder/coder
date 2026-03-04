import schema from "./chatModelOptionsGenerated.json";

/**
 * Describes a single configurable field for a chat model provider.
 * Generated from Go struct tags via `scripts/modeloptionsgen`.
 */
export interface FieldSchema {
	/** The JSON key used in API payloads (may use dot-notation for nested fields). */
	json_name: string;
	/** The corresponding Go struct field name. */
	go_name: string;
	/** The JSON Schema type of this field. */
	type: "string" | "integer" | "number" | "boolean" | "array" | "object";
	/** Human-readable description of the field. May be absent for some fields. */
	description?: string;
	/** Whether this field is required when configuring the provider. */
	required: boolean;
	/** Hint for how the frontend should render the input control. */
	input_type: "input" | "select" | "json";
	/** If present, the field value must be one of these options. */
	enum?: string[];
	/** If true, this field should not be rendered in admin UI forms. */
	hidden?: boolean;
}

/**
 * A group of fields belonging to a single provider or the general section.
 */
export interface ProviderSchema {
	fields: FieldSchema[];
}

/**
 * Top-level schema describing all configurable chat model options.
 *
 * - `general` contains provider-independent fields (e.g. temperature).
 * - `providers` maps canonical provider names to their specific fields.
 * - `provider_aliases` maps alternate names to canonical provider names
 *   (e.g. "azure" → "openai").
 */
export interface ModelOptionsSchema {
	general: ProviderSchema;
	providers: Record<string, ProviderSchema>;
	provider_aliases: Record<string, string>;
}

/** The imported schema, typed as {@link ModelOptionsSchema}. */
export const modelOptionsSchema: ModelOptionsSchema =
	schema as ModelOptionsSchema;

/**
 * Get the general (provider-independent) fields such as temperature
 * and max_output_tokens.
 */
export function getGeneralFields(): FieldSchema[] {
	return modelOptionsSchema.general.fields;
}

/**
 * Get provider-specific fields for a given provider name.
 * Handles aliases (e.g. "azure" → "openai", "bedrock" → "anthropic").
 * Returns an empty array for unknown providers.
 */
export function getProviderFields(provider: string): FieldSchema[] {
	const resolved = resolveProvider(provider);
	return modelOptionsSchema.providers[resolved]?.fields ?? [];
}

/**
 * Resolve a provider name through the alias table.
 * If the name is an alias it returns the canonical provider;
 * otherwise the original name is returned unchanged.
 *
 * @example
 * resolveProvider("azure")   // "openai"
 * resolveProvider("bedrock") // "anthropic"
 * resolveProvider("openai")  // "openai"
 */
export function resolveProvider(provider: string): string {
	return modelOptionsSchema.provider_aliases[provider] ?? provider;
}

/**
 * Get all canonical provider names (excludes aliases).
 * The order matches the JSON schema and is not guaranteed to be stable
 * across regenerations.
 */
export function getProviderNames(): string[] {
	return Object.keys(modelOptionsSchema.providers);
}

/**
 * Check whether a provider is known, either as a canonical name or an alias.
 */
export function isKnownProvider(provider: string): boolean {
	const resolved = resolveProvider(provider);
	return resolved in modelOptionsSchema.providers;
}

/**
 * Convert a snake_case segment to camelCase.
 * Only the first character after each underscore is uppercased;
 * the leading character stays lowercase.
 */
export function snakeToCamel(s: string): string {
	return s.replace(/_([a-z0-9])/g, (_, ch: string) => ch.toUpperCase());
}

/**
 * Convert a dot-notation `json_name` into a form field key namespaced
 * under the given provider.
 *
 * Each dot-separated segment is converted from snake_case to camelCase
 * and joined back with dots, then prefixed with the provider name.
 *
 * This bridges between the JSON schema (snake_case, flat `json_name`)
 * and a typical React form state tree (camelCase, dot-separated paths).
 *
 * @example
 * toFormFieldKey("anthropic", "thinking.budget_tokens")
 * // "anthropic.thinking.budgetTokens"
 *
 * toFormFieldKey("openai", "max_completion_tokens")
 * // "openai.maxCompletionTokens"
 */
export function toFormFieldKey(provider: string, jsonName: string): string {
	const camelSegments = jsonName.split(".").map(snakeToCamel);
	return `${provider}.${camelSegments.join(".")}`;
}

/** Get only the visible (non-hidden) fields for a provider. */
export function getVisibleProviderFields(provider: string): FieldSchema[] {
	return getProviderFields(provider).filter((f) => !f.hidden);
}

/** Get only the visible (non-hidden) general fields. */
export function getVisibleGeneralFields(): FieldSchema[] {
	return getGeneralFields().filter((f) => !f.hidden);
}
