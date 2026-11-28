/**
 * A random id for an annotation: 16 random bytes as hex. Ids only need to
 * be unique within a chat, and `getRandomValues` works in every context,
 * including the plain-HTTP previews where `crypto.randomUUID` is missing.
 */
export function randomId(): string {
	const bytes = crypto.getRandomValues(new Uint8Array(16));
	return Array.from(bytes, (byte) => byte.toString(16).padStart(2, "0")).join(
		"",
	);
}
