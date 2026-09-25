/**
 * A random id for an annotation. `crypto.randomUUID` is only defined in
 * secure contexts, and previews on plain-HTTP deployments are not one, so
 * fall back to `getRandomValues`, which every context has. Ids only need
 * to be unique within a chat, not globally.
 */
export function randomId(): string {
	if (typeof crypto.randomUUID === "function") {
		return crypto.randomUUID();
	}
	const bytes = crypto.getRandomValues(new Uint8Array(16));
	return Array.from(bytes, (byte) => byte.toString(16).padStart(2, "0")).join(
		"",
	);
}
