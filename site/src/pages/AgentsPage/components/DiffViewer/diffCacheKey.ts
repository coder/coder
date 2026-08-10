/**
 * Build a stable worker-pool cache key prefix for `@pierre/diffs`.
 *
 * We use React Query's `dataUpdatedAt` as the invalidation token instead of a
 * component-local counter. That timestamp survives component remounts, so a
 * freshly fetched diff cannot accidentally reuse a highlighted AST cached for
 * an older diff body.
 */
export const getDiffCacheKeyPrefix = (
	prefix: string,
	dataUpdatedAt: number,
): string => `${prefix}-${dataUpdatedAt}`;

/**
 * The @pierre/diffs worker pool keys cached ASTs by file name alone, so
 * hash the patch text to keep different diff bodies for the same path on
 * distinct keys, stable across re-renders and remounts.
 */
export const getContentCacheKeyPrefix = (text: string): string => {
	// FNV-1a. The key only needs to separate different patch bodies for the
	// same path while an older AST is cached, so a 32-bit checksum is plenty;
	// crypto.subtle is async and cannot run in the render path.
	let hash = 0x811c9dc5;
	for (let i = 0; i < text.length; i++) {
		hash ^= text.charCodeAt(i);
		hash = Math.imul(hash, 0x01000193);
	}
	return `content-${(hash >>> 0).toString(16)}`;
};
