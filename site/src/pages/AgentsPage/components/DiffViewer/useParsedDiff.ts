import type { FileDiffMetadata } from "@pierre/diffs";
import { parsePatchFiles } from "@pierre/diffs";
import { useState } from "react";

/**
 * Parse a unified diff string into an array of per-file metadata.
 *
 * Both `LocalDiffPanel` and `RemoteDiffPanel` need the same
 * `parsePatchFiles(…).flatMap(p => p.files)` pipeline. This hook
 * centralises that logic and keeps the panels focused on layout.
 *
 * Results are cached in state so that `parsePatchFiles` is only
 * called when the inputs actually change. Without this, every
 * unrelated re-render of the parent (e.g. scroll-target state in
 * `RemoteDiffPanel`) would re-parse the full unified diff.
 *
 * The "set state during render" pattern is the official React
 * approach for adjusting state based on changed props/arguments
 * and is compatible with the React Compiler (unlike a ref-based
 * cache which triggers "Cannot access refs during render").
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
