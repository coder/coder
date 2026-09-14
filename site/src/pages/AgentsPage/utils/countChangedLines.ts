import type { FileDiffMetadata } from "@pierre/diffs";

/** Sums added and deleted line counts across every hunk in a file diff. */
export function countChangedLines(fileDiff: FileDiffMetadata) {
	let additions = 0;
	let deletions = 0;
	for (const hunk of fileDiff.hunks) {
		additions += hunk.additionLines;
		deletions += hunk.deletionLines;
	}
	return { additions, deletions };
}
