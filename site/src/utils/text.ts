/**
 * Clamp a message body to a maximum character length, adding an
 * ellipsis when truncated.
 */
export const clampContent = (text: string, maxLen: number): string => {
	const trimmed = text.trim();
	if (trimmed.length <= maxLen) {
		return trimmed;
	}
	// Use Array.from to split on code points rather than UTF-16 code
	// units. A plain String.slice can cut a surrogate pair in half,
	// producing a lone high surrogate rendered as U+FFFD.
	const codePoints = Array.from(trimmed);
	if (codePoints.length <= maxLen) {
		return trimmed;
	}
	return `${codePoints.slice(0, maxLen).join("").trimEnd()}…`;
};

/**
 * Returns true when the text is long enough that clampContent would
 * actually truncate it. Uses the same trim+code-point count so callers
 * never offer a "see more" control for text that clampContent returns
 * unchanged.
 */
export const exceedsClampThreshold = (
	text: string,
	maxLen: number,
): boolean => {
	const trimmed = text.trim();
	if (trimmed.length <= maxLen) {
		return false;
	}
	return Array.from(trimmed).length > maxLen;
};
