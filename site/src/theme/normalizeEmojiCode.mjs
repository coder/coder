/**
 * Normalizes hex codes for description lookup across filenames that omit the
 * emoji presentation selector.
 * @param {string} code
 * @returns {string}
 */
export const normalizeEmojiCode = (code) =>
	code.toLowerCase().replace(/-fe0f/g, "");
