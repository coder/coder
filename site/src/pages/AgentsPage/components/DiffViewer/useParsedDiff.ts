import type { FileDiffMetadata } from "@pierre/diffs";
import { parsePatchFiles } from "@pierre/diffs";
import { useState } from "react";

/**
 * Parse a unified diff string into an array of per-file metadata.
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
	const [cache, setCache] = useState<{
		key: string;
		result: FileDiffMetadata[];
	}>({ key: "", result: [] });

	const key = `${diffString ?? ""}\0${cacheKeyPrefix ?? ""}`;
	if (cache.key !== key) {
		let result: FileDiffMetadata[];
		if (!diffString) {
			result = [];
		} else {
			try {
				const patches = parsePatchFiles(diffString, cacheKeyPrefix);
				result = patches.flatMap((p) => p.files);
			} catch (e) {
				console.error("Failed to parse diff:", e);
				result = [];
			}
		}
		setCache({ key, result });
		return result;
	}

	return cache.result;
}
