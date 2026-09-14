import type { Schema } from "yup";

export const asRecord = (value: unknown): Record<string, unknown> | null => {
	if (!value || typeof value !== "object" || Array.isArray(value)) {
		return null;
	}
	return value as Record<string, unknown>;
};

export const asString = (value: unknown): string =>
	typeof value === "string" ? value : "";

/**
 * Type-narrowing wrapper around a Yup schema. Returns `true`
 * (and narrows `value` to `T`) when `value` satisfies the
 * schema. Strict mode is always enabled to prevent silent
 * type coercion.
 */
export const isValid = <T>(schema: Schema<T>, value: unknown): value is T =>
	schema.isValidSync(value, { strict: true });

export const asNumber = (
	value: unknown,
	options?: { readonly parseString?: boolean },
): number | undefined => {
	if (typeof value === "number" && Number.isFinite(value)) {
		return value;
	}
	if (options?.parseString && typeof value === "string") {
		const parsed = Number(value);
		if (Number.isFinite(parsed)) {
			return parsed;
		}
	}
	return undefined;
};
