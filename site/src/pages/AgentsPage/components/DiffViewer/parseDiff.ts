import type { FileDiffMetadata } from "@pierre/diffs";
import { parsePatchFiles } from "@pierre/diffs";
import { getContentCacheKey } from "./diffCacheKey";

// CodeView throws on duplicate item ids, so repeated post-image paths
// collapse to their first occurrence. Exported for tests.
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
 * Parses a unified or git diff string into per-file metadata; empty input
 * yields []. Each file is keyed by a hash of its own render inputs so
 * unchanged files keep their highlight cache hits.
 */
export function parseDiffString(
	diffString: string | undefined | null,
): FileDiffMetadata[] {
	if (!diffString) return [];
	const files = parsePatchFiles(diffString).flatMap((p) => p.files);
	for (const file of files) {
		stampCacheKey(file);
	}
	return dedupeFilesByName(files);
}

/**
 * Stamps `file.cacheKey` with a hash of the inputs the worker reads when
 * highlighting; callers that mutate a parsed file must restamp it.
 */
export function stampCacheKey(file: FileDiffMetadata): void {
	file.cacheKey = getContentCacheKey(serializeRenderInputs(file));
}

// Everything the renderer walks when building the highlighted AST: equal
// serializations are equal render inputs.
const serializeRenderInputs = (file: FileDiffMetadata): string =>
	JSON.stringify([
		file.name,
		file.prevName,
		file.lang,
		file.hunks,
		file.additionLines,
		file.deletionLines,
	]);
