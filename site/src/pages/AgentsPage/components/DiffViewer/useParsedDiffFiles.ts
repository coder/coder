import { type FileDiffMetadata, parsePatchFiles } from "@pierre/diffs";

const EMPTY_PARSED_FILES: readonly FileDiffMetadata[] = [];
const PARSED_DIFF_FILES_CACHE_LIMIT = 64;
const parsedDiffFilesCache = new Map<string, readonly FileDiffMetadata[]>();

function getParsedDiffFilesCacheKey(
	diff?: string,
	cacheKeyPrefix?: string,
): string | null {
	if (!diff) {
		return null;
	}
	return `${cacheKeyPrefix ?? ""}\u0000${diff}`;
}

function setCachedParsedDiffFiles(
	cacheKey: string,
	parsedFiles: readonly FileDiffMetadata[],
): readonly FileDiffMetadata[] {
	parsedDiffFilesCache.set(cacheKey, parsedFiles);
	if (parsedDiffFilesCache.size > PARSED_DIFF_FILES_CACHE_LIMIT) {
		const oldestKey = parsedDiffFilesCache.keys().next().value;
		if (oldestKey) {
			parsedDiffFilesCache.delete(oldestKey);
		}
	}
	return parsedFiles;
}

/**
 * Parse unified diff content only when the diff payload or cache key changes.
 * This avoids rebuilding parsed file metadata on unrelated re-renders.
 */
export function useParsedDiffFiles(
	diff?: string,
	cacheKeyPrefix?: string,
): readonly FileDiffMetadata[] {
	const cacheKey = getParsedDiffFilesCacheKey(diff, cacheKeyPrefix);
	if (!cacheKey) {
		return EMPTY_PARSED_FILES;
	}

	const cachedParsedFiles = parsedDiffFilesCache.get(cacheKey);
	if (cachedParsedFiles) {
		return cachedParsedFiles;
	}

	try {
		return setCachedParsedDiffFiles(
			cacheKey,
			parsePatchFiles(diff ?? "", cacheKeyPrefix).flatMap(
				(patch) => patch.files,
			),
		);
	} catch {
		return setCachedParsedDiffFiles(cacheKey, EMPTY_PARSED_FILES);
	}
}
