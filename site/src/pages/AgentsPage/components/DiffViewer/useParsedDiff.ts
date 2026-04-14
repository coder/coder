import type { FileDiffMetadata } from "@pierre/diffs";
import { parsePatchFiles } from "@pierre/diffs";

/**
 * Parse a unified diff string into an array of per-file metadata.
 *
 * Both `LocalDiffPanel` and `RemoteDiffPanel` need the same
 * `parsePatchFiles(…).flatMap(p => p.files)` pipeline. This hook
 * centralises that logic and keeps the panels focused on layout.
 *
 * @param diffString  Raw unified diff (may be null/undefined).
 * @param cacheKeyPrefix  Optional cache-key prefix forwarded to the
 *   `@pierre/diffs` worker-pool LRU cache. When supplied the prefix
 *   must change whenever the diff payload changes so highlighted ASTs
 *   are not reused across different diff bodies.
 */
export function useParsedDiff(
	diffString: string | undefined | null,
	cacheKeyPrefix?: string,
): FileDiffMetadata[] {
	if (!diffString) {
		return [];
	}
	try {
		const patches = parsePatchFiles(diffString, cacheKeyPrefix);
		return patches.flatMap((p) => p.files);
	} catch (e) {
		console.error("Failed to parse diff:", e);
		return [];
	}
}
