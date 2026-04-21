import type { FileDiffMetadata } from "@pierre/diffs";
import { parsePatchFiles } from "@pierre/diffs";
import { useMemo } from "react";

/**
 * Parse a unified diff string into an array of per-file metadata.
 *
 * Both `LocalDiffPanel` and `RemoteDiffPanel` need the same
 * `parsePatchFiles(…).flatMap(p => p.files)` pipeline. This hook
 * centralises that logic and keeps the panels focused on layout.
 *
 * Note: This uses `useMemo` despite being in a React Compiler-managed
 * directory. `parsePatchFiles` is an external function from
 * `@pierre/diffs` — the compiler cannot prove its purity via static
 * analysis, so the call would run on every render without explicit
 * memoization. For large unified diffs this is a measurable cost.
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
	return useMemo(() => {
		if (!diffString) return [];
		try {
			return parsePatchFiles(diffString, cacheKeyPrefix).flatMap(
				(p) => p.files,
			);
		} catch (e) {
			console.error("Failed to parse diff:", e);
			return [];
		}
	}, [diffString, cacheKeyPrefix]);
}
