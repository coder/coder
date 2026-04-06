/**
 * Single-pass diff stat counter. Avoids allocating a temporary
 * array from split("\n") by walking the string with indexOf.
 */
export function countDiffStatLines(unifiedDiff: string): {
	additions: number;
	deletions: number;
} {
	let additions = 0;
	let deletions = 0;
	let pos = 0;

	while (pos < unifiedDiff.length) {
		const nextNewline = unifiedDiff.indexOf("\n", pos);
		const lineEnd = nextNewline === -1 ? unifiedDiff.length : nextNewline;

		const ch = unifiedDiff[pos];
		if (ch === "+") {
			// Exclude "+++ " header lines.
			if (unifiedDiff[pos + 1] !== "+" || unifiedDiff[pos + 2] !== "+") {
				additions++;
			}
		} else if (ch === "-") {
			// Exclude "--- " header lines.
			if (unifiedDiff[pos + 1] !== "-" || unifiedDiff[pos + 2] !== "-") {
				deletions++;
			}
		}

		pos = lineEnd + 1;
	}

	return { additions, deletions };
}
