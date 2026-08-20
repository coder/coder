/**
 * The @pierre/diffs worker pool keys cached ASTs by each file's `cacheKey`,
 * which defaults to the file name (or prevName:name for renames) when
 * unset; hash the parsed content so different diff bodies for the same
 * path land on distinct keys, stable across re-renders and remounts.
 */
export const getContentCacheKey = (text: string): string => {
	// FNV-1a plus the text length. The key only needs to separate different
	// diff bodies while an older AST is cached, so a 32-bit checksum is
	// plenty; crypto.subtle is async and cannot run in the render path.
	let hash = 0x811c9dc5;
	for (let i = 0; i < text.length; i++) {
		hash ^= text.charCodeAt(i);
		hash = Math.imul(hash, 0x01000193);
	}
	return `content-${(hash >>> 0).toString(16)}-${text.length.toString(16)}`;
};
