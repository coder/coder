/**
 * Build a stable worker-pool cache key for `@pierre/diffs` that changes
 * whenever the diff contents change, even across component remounts.
 *
 * The upstream library only keys its highlighted diff cache by `cacheKey`, so
 * callers must update that key whenever the diff text changes. We derive the
 * key from a small deterministic hash of the diff contents instead of using a
 * component-local counter, which can reset on remount and collide with stale
 * cached ASTs.
 */
export const getDiffCacheKeyPrefix = (
	prefix: string,
	diffContent: string,
): string => {
	let hash = 0x811c9dc5;
	for (let i = 0; i < diffContent.length; i++) {
		hash ^= diffContent.charCodeAt(i);
		hash = Math.imul(hash, 0x01000193);
	}
	return `${prefix}-${diffContent.length}-${(hash >>> 0).toString(36)}`;
};
