/**
 * Returns true if the given BMP codepoint is a visible character that
 * should be preserved in prompt text. This mirrors the backend Go
 * isVisible function in coderd/x/chatd/sanitize.go — both use the
 * same codepoint list with the same polarity (true = visible).
 *
 * All codepoints in this list are in the Basic Multilingual Plane,
 * so charCodeAt() is safe to use without surrogate pair handling.
 */
function isVisible(code: number): boolean {
	// Individual invisible codepoints.
	if (
		code === 0x00ad || // Soft hyphen
		code === 0x034f || // Combining grapheme joiner
		code === 0x061c || // Arabic letter mark
		code === 0x180e || // Mongolian vowel separator
		code === 0xfeff // Byte order mark / zero-width no-break space
	) {
		return false;
	}
	// Zero-width and directional marks: U+200B, U+200D–U+200F.
	// U+200C (ZWNJ) is deliberately excluded — it is required for
	// correct rendering of Persian, Urdu, and Kurdish scripts.
	if (code === 0x200b || (code >= 0x200d && code <= 0x200f)) {
		return false;
	}
	// Bidi embedding/override: U+202A–U+202E.
	if (code >= 0x202a && code <= 0x202e) {
		return false;
	}
	// Invisible operators: U+2060–U+2064.
	if (code >= 0x2060 && code <= 0x2064) {
		return false;
	}
	// Bidi isolates: U+2066–U+2069.
	if (code >= 0x2066 && code <= 0x2069) {
		return false;
	}
	// Deprecated format characters (U+206A–U+206F): inhibit
	// swapping, activate swapping, inhibit/activate Arabic form
	// shaping, national digit shapes, nominal digit shapes.
	if (code >= 0x206a && code <= 0x206f) {
		return false;
	}
	// Interlinear annotation characters (U+FFF9–U+FFFB):
	// annotation anchor, separator, terminator.
	if (code >= 0xfff9 && code <= 0xfffb) {
		return false;
	}
	return true;
}

/**
 * Detects invisible Unicode characters that could hide prompt
 * injection content. Returns the count found, or 0 if clean.
 */
export function countInvisibleCharacters(text: string): number {
	let count = 0;
	for (let i = 0; i < text.length; i++) {
		if (!isVisible(text.charCodeAt(i))) {
			count++;
		}
	}
	return count;
}
