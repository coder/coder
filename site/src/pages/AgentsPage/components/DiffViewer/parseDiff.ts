import type { FileDiffMetadata } from "@pierre/diffs";
import { parsePatchFiles } from "@pierre/diffs";
import { getContentCacheKey } from "./diffCacheKey";

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
 * Parses a unified or git diff string into per-file metadata; empty input
 * yields []. Each file is keyed by a hash of its own render inputs, so
 * unchanged files keep hitting the worker-pool highlight cache across
 * re-parses while changed files miss it. With `dedupe` (default), repeated
 * post-image paths collapse to their first occurrence; pass false for
 * single-file synthetic diffs where repeats are expected, not malformed.
 */
export function parseDiffString(
	diffString: string | undefined | null,
	dedupe = true,
): FileDiffMetadata[] {
	if (!diffString) return [];
	const files = parsePatchFiles(diffString).flatMap((p) => p.files);
	for (const file of files) {
		stampCacheKey(file);
	}
	return dedupe ? dedupeFilesByName(files) : files;
}

/**
 * Stamps `file.cacheKey` with a hash of the inputs the worker reads when
 * building the highlighted AST. The worker pool caches highlighted ASTs by
 * cacheKey alone, so callers that clone or edit a parsed file must restamp
 * the result or it renders against the unmutated cached AST.
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
