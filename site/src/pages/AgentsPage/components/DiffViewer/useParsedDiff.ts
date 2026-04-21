import type { FileDiffMetadata } from "@pierre/diffs";
import { parsePatchFiles } from "@pierre/diffs";
import { useMemo } from "react";

// Uses explicit useMemo despite the React Compiler scope because
// parsePatchFiles is external to the compiler's static analysis.
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
