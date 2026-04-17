import {
	type FileDiffMetadata,
	type ParsedPatch,
	parsePatchFiles,
} from "@pierre/diffs";

const EMPTY_PARSED_FILES: readonly FileDiffMetadata[] = [];
const PARSED_DIFF_FILES_CACHE_LIMIT = 64;

interface ParsedDiffFilesCacheEntry {
	parsedPatches: readonly ParsedPatch[];
	parsedFiles: readonly FileDiffMetadata[];
	cacheKeyPrefix?: string;
}

const parsedDiffFilesCache = new Map<string, ParsedDiffFilesCacheEntry>();

function getParsedDiffFilesCacheKey(diff?: string): string | null {
	if (!diff) {
		return null;
	}
	return diff;
}

function getFileCacheKey(
	cacheKeyPrefix: string | undefined,
	patchIndex: number,
	fileIndex: number,
): string | undefined {
	if (cacheKeyPrefix == null) {
		return undefined;
	}
	return `${cacheKeyPrefix}-${patchIndex}-${fileIndex}`;
}

function getParsedFilesForCacheKeyPrefix(
	parsedPatches: readonly ParsedPatch[],
	cacheKeyPrefix?: string,
): readonly FileDiffMetadata[] {
	return parsedPatches.flatMap((patch, patchIndex) =>
		patch.files.map((file, fileIndex) => {
			const cacheKey = getFileCacheKey(cacheKeyPrefix, patchIndex, fileIndex);
			if (file.cacheKey === cacheKey) {
				return file;
			}
			return { ...file, cacheKey };
		}),
	);
}

function setCachedParsedDiffFiles(
	cacheKey: string,
	cacheEntry: ParsedDiffFilesCacheEntry,
): readonly FileDiffMetadata[] {
	parsedDiffFilesCache.set(cacheKey, cacheEntry);
	if (parsedDiffFilesCache.size > PARSED_DIFF_FILES_CACHE_LIMIT) {
		const oldestKey = parsedDiffFilesCache.keys().next().value;
		if (oldestKey) {
			parsedDiffFilesCache.delete(oldestKey);
		}
	}
	return cacheEntry.parsedFiles;
}

/**
 * Parse each diff payload once, then only refresh the worker-pool cache keys
 * when the caller changes `cacheKeyPrefix`.
 */
export function useParsedDiffFiles(
	diff?: string,
	cacheKeyPrefix?: string,
): readonly FileDiffMetadata[] {
	const cacheKey = getParsedDiffFilesCacheKey(diff);
	if (!cacheKey) {
		return EMPTY_PARSED_FILES;
	}

	const cachedParsedFiles = parsedDiffFilesCache.get(cacheKey);
	if (cachedParsedFiles) {
		if (cachedParsedFiles.cacheKeyPrefix === cacheKeyPrefix) {
			return cachedParsedFiles.parsedFiles;
		}
		return setCachedParsedDiffFiles(cacheKey, {
			parsedPatches: cachedParsedFiles.parsedPatches,
			parsedFiles: getParsedFilesForCacheKeyPrefix(
				cachedParsedFiles.parsedPatches,
				cacheKeyPrefix,
			),
			cacheKeyPrefix,
		});
	}

	try {
		const parsedPatches = parsePatchFiles(diff ?? "", cacheKeyPrefix);
		return setCachedParsedDiffFiles(cacheKey, {
			parsedPatches,
			parsedFiles: parsedPatches.flatMap((patch) => patch.files),
			cacheKeyPrefix,
		});
	} catch {
		return setCachedParsedDiffFiles(cacheKey, {
			parsedPatches: [],
			parsedFiles: EMPTY_PARSED_FILES,
			cacheKeyPrefix,
		});
	}
}
