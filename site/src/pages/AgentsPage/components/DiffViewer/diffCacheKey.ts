/**
 * The @pierre/diffs worker pool only caches highlighted ASTs for files with
 * a `cacheKey`, and renderers compare keyed diffs by key instead of object
 * identity. Hash the parsed content so re-parsed but unchanged files keep
 * their cache hits and skip re-rendering, while different diff bodies for
 * the same path land on distinct keys.
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
