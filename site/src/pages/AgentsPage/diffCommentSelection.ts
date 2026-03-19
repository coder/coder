import type { SelectedLineRange } from "@pierre/diffs";

// -------------------------------------------------------------------
// Types
// -------------------------------------------------------------------

type AnnotationSide = "additions" | "deletions";

/**
 * Range reported by the @pierre/diffs library when a user selects
 * one or more lines. `endSide` is present only when the selection
 * crosses from one side to the other (e.g. deletions → additions in
 * a split diff).
 */
export interface LineSelectionRange {
	start: number;
	end: number;
	side?: AnnotationSide;
	endSide?: AnnotationSide;
}

/**
 * Internal state tracked for an active inline comment input.
 */
export interface CommentBoxState {
	fileName: string;
	startLine: number;
	endLine: number;
	side: AnnotationSide;
	endSide?: AnnotationSide;
}

// -------------------------------------------------------------------
// Pure helpers
// -------------------------------------------------------------------

/**
 * Compute the comment box state from a selection range reported by
 * the diff library.
 *
 * Returns:
 * - `null`        when the range is null (selection cleared).
 * - `"ignore"`    when the range is a same-side single-line click
 *                 (these are handled by `handleLineNumberClick`
 *                 instead).
 * - A `CommentBoxState` otherwise.
 */
export function commentBoxFromRange(
	fileName: string,
	range: LineSelectionRange | null,
): CommentBoxState | null | "ignore" {
	if (!range) return null;

	// A selection is truly single-line only when start === end AND
	// there is no endSide. Cross-side selections in split view can
	// have start === end with different sides (e.g. deletion line 16
	// to addition line 16 spans two visual rows).
	if (range.start === range.end && !range.endSide) return "ignore";

	const side = range.side ?? "additions";
	const endSide = range.endSide;
	return {
		fileName,
		startLine: Math.min(range.start, range.end),
		endLine: Math.max(range.start, range.end),
		side,
		...(endSide && { endSide }),
	};
}

/**
 * The side on which the inline annotation (comment input) should be
 * rendered. For cross-side selections the annotation appears on the
 * end side so it sits visually at the bottom of the highlighted
 * range.
 */
export function annotationSideForBox(box: CommentBoxState): AnnotationSide {
	return box.endSide ?? box.side;
}

/**
 * Build the `SelectedLineRange` that the diff library needs to
 * visually highlight the selected lines.
 */
export function selectedLinesForBox(box: CommentBoxState): SelectedLineRange {
	return {
		start: box.startLine,
		end: box.endLine,
		side: box.side,
		...(box.endSide && { endSide: box.endSide }),
	};
}
