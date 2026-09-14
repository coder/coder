/**
 * Coerces a module variable's `default` into placeholder text.
 *
 * `TemplateBuilderModuleVariable.default` is generated as
 * `Record<string, string>` because every `json.RawMessage` field maps to that
 * type, but the actual wire value is a JSON scalar (string, number, or
 * boolean). The parameter is typed `unknown` so the generated value is accepted
 * without a cast.
 *
 * An empty-string default returns `undefined` so callers fall back to their
 * existing Required/Optional hint. Object or array defaults also return
 * `undefined` since they have no meaningful placeholder representation.
 */
export function defaultPlaceholder(value: unknown): string | undefined {
	if (typeof value === "number" || typeof value === "boolean") {
		return String(value);
	}
	if (typeof value === "string" && value !== "") {
		return value;
	}
	return undefined;
}
