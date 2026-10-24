import { getPathBasename, getPathDirname } from "../../utils/path";

const directorySegments = (path: string) =>
	getPathDirname(path).split("/").filter(Boolean);

const runMatches = (
	segments: readonly string[],
	start: number,
	run: readonly string[],
) => run.every((segment, offset) => segments[start + offset] === segment);

/**
 * Whether `other` has `run` where a hint for it would read the same: at its
 * start for a hint anchored at the start, at its end for one anchored at the
 * end, anywhere for a hint with ellipses on both sides.
 */
const hasRun = (
	other: readonly string[],
	run: readonly string[],
	atStart: boolean,
	atEnd: boolean,
) => {
	if (atStart && atEnd) {
		return other.length === run.length && runMatches(other, 0, run);
	}
	if (atStart) {
		return runMatches(other, 0, run);
	}
	if (atEnd) {
		return (
			other.length >= run.length &&
			runMatches(other, other.length - run.length, run)
		);
	}
	for (let start = 0; start + run.length <= other.length; start++) {
		if (runMatches(other, start, run)) {
			return true;
		}
	}
	return false;
};

const formatHint = (run: readonly string[], atStart: boolean, atEnd: boolean) =>
	[atStart ? "" : "…", run.join("/"), atEnd ? "" : "…"]
		.filter(Boolean)
		.join("/");

/**
 * Directory hints for tabs that share a basename, keyed by path. A hint is
 * the shortest run of parent directories, below the group's common
 * ancestor, that tells the tab apart from the others in its group; "…"
 * stands for the directories it leaves out.
 */
export const getTabHints = (
	paths: readonly string[],
): ReadonlyMap<string, string> => {
	const groups = new Map<string, string[]>();
	for (const path of paths) {
		const basename = getPathBasename(path);
		groups.set(basename, [...(groups.get(basename) ?? []), path]);
	}

	const hints = new Map<string, string>();
	for (const group of groups.values()) {
		if (group.length < 2) {
			continue;
		}
		const directories = group.map(directorySegments);
		const [first = []] = directories;
		let shared = 0;
		while (
			shared < first.length &&
			directories.every((segments) => segments[shared] === first[shared])
		) {
			shared++;
		}
		const relative = directories.map((segments) => segments.slice(shared));

		for (const [index, path] of group.entries()) {
			const own = relative[index] ?? [];
			const others = relative.filter((_, other) => other !== index);
			hints.set(
				path,
				own.length === 0
					? (directories[index]?.at(-1) ?? "/")
					: shortestHint(own, others),
			);
		}
	}
	return hints;
};

const shortestHint = (
	own: readonly string[],
	others: readonly (readonly string[])[],
) => {
	for (let length = 1; length <= own.length; length++) {
		// Directories nearest the file read best, so try those first.
		for (let start = own.length - length; start >= 0; start--) {
			const run = own.slice(start, start + length);
			const atStart = start === 0;
			const atEnd = start + length === own.length;
			if (!others.some((other) => hasRun(other, run, atStart, atEnd))) {
				return formatHint(run, atStart, atEnd);
			}
		}
	}
	return own.join("/");
};
