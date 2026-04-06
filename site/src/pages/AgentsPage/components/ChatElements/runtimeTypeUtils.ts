export const asRecord = (value: unknown): Record<string, unknown> | null => {
	if (!value || typeof value !== "object" || Array.isArray(value)) {
		return null;
	}
	return value as Record<string, unknown>;
};

export const asString = (value: unknown): string =>
	typeof value === "string" ? value : "";

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
