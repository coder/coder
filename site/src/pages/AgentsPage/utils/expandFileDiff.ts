import {
	type FileContents,
	type FileDiffMetadata,
	processFile,
} from "@pierre/diffs";

export const MAX_EXPANDABLE_FILE_SIZE = 512 * 1024; // 512 KB

/**
 * Takes a single file's patch string and the full old/new file contents,
 * returns an enriched FileDiffMetadata with isPartial: false.
 * This enables the @pierre/diffs library's native expand-context feature.
 *
 * @param fileName - The file name (used for language detection)
 * @param patchString - The single-file unified diff string (the portion
 *   of the full diff that belongs to this file, including the diff --git header)
 * @param oldContents - Full old file contents, or null for new files
 * @param newContents - Full new file contents, or null for deleted files
 * @param cacheKey - Optional cache key for the worker pool
 * @returns Enriched FileDiffMetadata, or null if parsing fails
 */
export function expandFileDiff(
	fileName: string,
	patchString: string,
	oldContents: string | null,
	newContents: string | null,
	cacheKey?: string,
): FileDiffMetadata | null {
	// For new files (null old), use empty string so processFile sees both
	// sides and can set isPartial: false. Same for deleted files (null new).
	const oldFile: FileContents = {
		name: fileName,
		contents: oldContents ?? "",
	};
	const newFile: FileContents = {
		name: fileName,
		contents: newContents ?? "",
	};

	const result = processFile(patchString, {
		oldFile,
		newFile,
		cacheKey,
		isGitDiff: true,
	});

	return result ?? null;
}

/**
 * Checks if a file is eligible for expansion based on content size.
 */
export function isExpandable(
	oldContents: string | null,
	newContents: string | null,
): boolean {
	if (oldContents !== null && oldContents.length > MAX_EXPANDABLE_FILE_SIZE) {
		return false;
	}
	if (newContents !== null && newContents.length > MAX_EXPANDABLE_FILE_SIZE) {
		return false;
	}
	return true;
}
