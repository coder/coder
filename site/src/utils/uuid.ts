export const isUUID = (text: string) => {
	const UUID =
		/^[0-9a-f]{8}-[0-9a-f]{4}-[0-5][0-9a-f]{3}-[89ab][0-9a-f]{3}-[0-9a-f]{12}$/i;
	return UUID.test(text);
};

/**
 * Generate a random RFC 4122 version 4 UUID.
 *
 * Uses `crypto.randomUUID()` when the runtime provides it. Falls back to
 * generating random bytes with `crypto.getRandomValues()` and formatting
 * them as a v4 UUID for environments where `randomUUID` is unavailable
 * (for example, non-secure contexts).
 */
export const generateUUID = (): string => {
	if (typeof crypto.randomUUID === "function") {
		return crypto.randomUUID();
	}

	const bytes = crypto.getRandomValues(new Uint8Array(16));
	// Set the version (4) and variant (RFC 4122) bits.
	bytes[6] = (bytes[6] & 0x0f) | 0x40;
	bytes[8] = (bytes[8] & 0x3f) | 0x80;

	const hex: string[] = [];
	for (const byte of bytes) {
		hex.push(byte.toString(16).padStart(2, "0"));
	}

	return [
		hex.slice(0, 4).join(""),
		hex.slice(4, 6).join(""),
		hex.slice(6, 8).join(""),
		hex.slice(8, 10).join(""),
		hex.slice(10, 16).join(""),
	].join("-");
};
