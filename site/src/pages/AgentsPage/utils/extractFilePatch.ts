/**
 * Extracts the unified diff section for a single file from a
 * multi-file unified diff string. The returned string includes
 * the `diff --git` header through to the end of the last hunk
 * for that file.
 *
 * Returns null if the file isn't found in the diff.
 */
export function extractFilePatch(
	fullDiff: string,
	fileName: string,
): string | null {
	// Split the full diff into per-file sections at each
	// `diff --git` boundary.
	const diffHeaders = [...fullDiff.matchAll(/^diff --git /gm)];
	if (diffHeaders.length === 0) return null;

	for (let i = 0; i < diffHeaders.length; i++) {
		const start = diffHeaders[i].index;
		if (start === undefined) continue;
		const end =
			i + 1 < diffHeaders.length ? diffHeaders[i + 1].index : fullDiff.length;
		const section = fullDiff.slice(start, end);

		// Check whether this section belongs to the requested file.
		// The diff header looks like: diff --git a/path b/path
		// We also check --- and +++ lines for renamed files.
		if (
			section.includes(`a/${fileName}`) ||
			section.includes(`b/${fileName}`)
		) {
			return section;
		}
	}

	return null;
}
