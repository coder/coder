import type { SelectedLineRange } from "@pierre/diffs";

// -------------------------------------------------------------------
// Types
// -------------------------------------------------------------------

type AnnotationSide = "additions" | "deletions";

/**
 * Range reported by the @pierre/diffs library when a user selects
 * one or more lines. `endSide` is present only when the selection
 * crosses from one side to the other (e.g. deletions -> additions in
 * a split diff).
 */
interface LineSelectionRange {
	start: number;
	end: number;
	side?: AnnotationSide;
	endSide?: AnnotationSide;
}

/**
 * Internal state tracked for an active inline comment input.
 *
 * `start`/`end` are the raw line numbers reported by the library,
 * each on its own side. For same-side selections both sides are
 * equal; for cross-side selections they refer to line numbers in
 * different file versions and MUST NOT be compared with min/max.
 */
export interface CommentBoxState {
	fileName: string;
	start: number;
	startSide: AnnotationSide;
	end: number;
	endSide: AnnotationSide;
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

	const startSide = range.side ?? "additions";
	const endSide = range.endSide ?? startSide;

	// Single-line same-side selections are handled by the line
	// number click handler, not the range selection handler.
	if (range.start === range.end && startSide === endSide) return "ignore";

	return {
		fileName,
		start: range.start,
		startSide,
		end: range.end,
		endSide,
	};
}

/**
 * The side on which the inline annotation (comment input) should be
 * rendered. For cross-side selections the annotation appears on the
 * end side so it sits visually at the bottom of the highlighted
 * range.
 */
export function annotationSideForBox(box: CommentBoxState): AnnotationSide {
	return box.endSide;
}

/**
 * The line number at which the annotation should be placed.
 */
export function annotationLineForBox(box: CommentBoxState): number {
	return box.end;
}

/**
 * Build the `SelectedLineRange` that the diff library needs to
 * visually highlight the selected lines. The library maps these
 * back to visual row indices internally, so we pass the raw
 * coordinates without normalization.
 */
export function selectedLinesForBox(box: CommentBoxState): SelectedLineRange {
	return {
		start: box.start,
		end: box.end,
		side: box.startSide,
		...(box.startSide !== box.endSide && { endSide: box.endSide }),
	};
}

/**
 * Derive a sensible single-side line range for content extraction
 * and the file reference chip.
 *
 * Same-side selections produce a normalized min..max range on that
 * side. Cross-side selections use the end point only, because start
 * and end refer to line numbers in different file versions and
 * cannot be meaningfully compared.
 */
export function contentRangeForBox(box: CommentBoxState): {
	startLine: number;
	endLine: number;
	side: AnnotationSide;
} {
	if (box.startSide === box.endSide) {
		return {
			startLine: Math.min(box.start, box.end),
			endLine: Math.max(box.start, box.end),
			side: box.startSide,
		};
	}
	// Cross-side: line numbers belong to different file versions.
	// Use the end point (where the annotation appears) as the
	// reference so the file chip is meaningful.
	return {
		startLine: box.end,
		endLine: box.end,
		side: box.endSide,
	};
}
