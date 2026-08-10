import type { FileDiffMetadata } from "@pierre/diffs";
import { parsePatchFiles } from "@pierre/diffs";
import { getContentCacheKeyPrefix } from "./diffCacheKey";

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

/**
 * Parses a unified or git diff string into per-file metadata, collapsing
 * repeated post-image paths. Each file is keyed by a hash of its own parsed
 * content (name, hunks, and line arrays) so unchanged files keep hitting the
 * worker-pool highlight cache across re-parses while changed files miss it.
 */
export function parseDiffString(
	diffString: string | undefined | null,
): FileDiffMetadata[] {
	if (!diffString) return [];
	const files = parsePatchFiles(diffString).flatMap((p) => p.files);
	for (const file of files) {
		file.cacheKey = getContentCacheKeyPrefix(
			`${file.name}\n${serializeHunks(file)}`,
		);
	}
	return dedupeFilesByName(files);
}

// The parsed hunks plus the file-level line arrays: everything the renderer
// walks when building the highlighted AST, so equal serializations are equal
// render inputs.
const serializeHunks = (file: FileDiffMetadata): string =>
	JSON.stringify([file.hunks, file.additionLines, file.deletionLines]);
