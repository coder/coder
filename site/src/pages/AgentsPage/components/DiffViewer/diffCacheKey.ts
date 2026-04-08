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
