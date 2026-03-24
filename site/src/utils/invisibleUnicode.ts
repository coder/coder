/**
 * Returns true if the given BMP codepoint is an invisible Unicode
 * character that should be flagged. This list matches the backend
 * Go implementation in coderd/x/chatd/sanitize.go.
 *
 * All codepoints in this list are in the Basic Multilingual Plane,
 * so charCodeAt() is safe to use without surrogate pair handling.
 */
function isInvisibleCodepoint(code: number): boolean {
	// Individual codepoints.
	if (
		code === 0x00ad || // Soft hyphen
		code === 0x034f || // Combining grapheme joiner
		code === 0x061c || // Arabic letter mark
		code === 0x180e || // Mongolian vowel separator
		code === 0xfeff // Byte order mark / zero-width no-break space
	) {
		return true;
	}
	// Zero-width and directional marks: U+200B–U+200F.
	if (code >= 0x200b && code <= 0x200f) {
		return true;
	}
	// Bidi embedding/override: U+202A–U+202E.
	if (code >= 0x202a && code <= 0x202e) {
		return true;
	}
	// Invisible operators: U+2060–U+2064.
	if (code >= 0x2060 && code <= 0x2064) {
		return true;
	}
	// Bidi isolates: U+2066–U+2069.
	if (code >= 0x2066 && code <= 0x2069) {
		return true;
	}
	return false;
}

/**
 * Detects invisible Unicode characters that could hide prompt
 * injection content. Returns the count found, or 0 if clean.
 */
export function countInvisibleCharacters(text: string): number {
	let count = 0;
	for (let i = 0; i < text.length; i++) {
		if (isInvisibleCodepoint(text.charCodeAt(i))) {
			count++;
		}
	}
	return count;
}
