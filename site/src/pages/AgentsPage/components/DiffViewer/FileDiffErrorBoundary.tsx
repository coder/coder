import type { FileDiffMetadata } from "@pierre/diffs";
import { AlertTriangleIcon } from "lucide-react";
import type { FC } from "react";
import { cn } from "utils/cn";

/**
 * Checks whether a {@link FileDiffMetadata} object is internally
 * consistent — i.e. every hunk's line indices fall within the
 * bounds of the file's `additionLines` and `deletionLines` arrays.
 *
 * When the upstream API returns a truncated or malformed diff,
 * `parsePatchFiles()` can produce metadata where the hunks claim
 * more lines than actually exist. Feeding that to `<FileDiff>`
 * causes an uncatchable throw inside the web component's async
 * highlight callback (`DiffHunksRenderer.processDiffResult`).
 * Validating up-front lets us show a fallback *before* the
 * renderer ever sees the bad data.
 */
export function isFileDiffValid(file: FileDiffMetadata): boolean {
	const addLen = file.additionLines?.length ?? 0;
	const delLen = file.deletionLines?.length ?? 0;

	for (const hunk of file.hunks) {
		// Hunk-level bounds: the hunk's starting index plus its
		// total line count (context + changes) must not exceed the
		// file's line arrays.
		if (hunk.additionLineIndex + hunk.additionCount > addLen) {
			return false;
		}
		if (hunk.deletionLineIndex + hunk.deletionCount > delLen) {
			return false;
		}

		// Content-level bounds: each ContextContent / ChangeContent
		// block within the hunk has its own indices into the file's
		// line arrays. These are what iterateOverDiff actually
		// walks, so an out-of-bounds here is the direct trigger for
		// the processDiffResult crash.
		for (const block of hunk.hunkContent) {
			if (block.type === "context") {
				if (block.additionLineIndex + block.lines > addLen) {
					return false;
				}
				if (block.deletionLineIndex + block.lines > delLen) {
					return false;
				}
			} else {
				if (block.additionLineIndex + block.additions > addLen) {
					return false;
				}
				if (block.deletionLineIndex + block.deletions > delLen) {
					return false;
				}
			}
		}
	}

	return true;
}

/**
 * Compact fallback rendered in place of a file diff that failed
 * validation. Explains what went wrong and that other files are
 * unaffected.
 */
export const FileDiffFallback: FC<{ fileName: string }> = ({ fileName }) => (
	<div
		data-testid="file-diff-fallback"
		className={cn(
			"flex flex-col gap-1 rounded px-3 py-2",
			"bg-surface-secondary text-xs",
		)}
	>
		<span className="flex items-center gap-2 text-content-primary font-medium">
			<AlertTriangleIcon className="size-3.5 shrink-0 text-content-warning" />
			Could not render diff for {fileName}
		</span>
		<span className="text-content-secondary pl-[22px]">
			The diff data for this file appears to be truncated or malformed, likely
			due to an upstream API issue. Other files are unaffected.
		</span>
	</div>
);
