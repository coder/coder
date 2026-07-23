import type { FileDiffMetadata } from "@pierre/diffs";
import { parsePatchFiles } from "@pierre/diffs";
import { useMemo } from "react";

// A single diff body can list the same post-image path more than once: the
// server may concatenate several `git diff` outputs, or one patch may carry
// multiple `diff --git` sections for the same file. Both the CodeView (which
// keys items by file name) and the file tree (which keys rows by path) require
// unique ids, and CodeView.addItem throws on a duplicate id, which tears down
// the entire diff view. Collapse repeats to their first occurrence so a
// malformed diff degrades gracefully instead of crashing. Exported for tests.
export function dedupeFilesByName(
	files: readonly FileDiffMetadata[],
): FileDiffMetadata[] {
	const seen = new Set<string>();
	const unique: FileDiffMetadata[] = [];
	const duplicates: string[] = [];
	for (const file of files) {
		if (seen.has(file.name)) {
			duplicates.push(file.name);
			continue;
		}
		seen.add(file.name);
		unique.push(file);
	}
	if (duplicates.length > 0) {
		console.warn(
			`Diff lists duplicate file paths; showing the first occurrence of each: ${duplicates.join(", ")}`,
		);
	}
	return unique;
}

// Uses explicit useMemo despite the React Compiler scope because
// parsePatchFiles is external to the compiler's static analysis.
export function useParsedDiff(
	diffString: string | undefined | null,
	cacheKeyPrefix?: string,
): FileDiffMetadata[] {
	return useMemo(() => {
		if (!diffString) return [];
		try {
			const files = parsePatchFiles(diffString, cacheKeyPrefix).flatMap(
				(p) => p.files,
			);
			return dedupeFilesByName(files);
		} catch (e) {
			console.error("Failed to parse diff:", e);
			return [];
		}
	}, [diffString, cacheKeyPrefix]);
}
