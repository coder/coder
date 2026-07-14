import type { DiffLineAnnotation } from "@pierre/diffs/react";
import { describe, expect, it } from "vitest";
import { annotationsVersion } from "./DiffViewer";

const annotation = (
	lineNumber: number,
	side: "additions" | "deletions" = "additions",
): DiffLineAnnotation<string> => ({
	side,
	lineNumber,
	metadata: "active-input",
});

describe("annotationsVersion", () => {
	it("is 0 when there are no annotations", () => {
		expect(annotationsVersion(undefined)).toBe(0);
		expect(annotationsVersion([])).toBe(0);
	});

	it("changes when the line moves but the count stays the same", () => {
		// The regression: a single active comment box moving between lines keeps
		// the count at 1, so a count-based version would not change and CodeView
		// would skip the update.
		expect(annotationsVersion([annotation(5)])).not.toBe(
			annotationsVersion([annotation(10)]),
		);
	});

	it("changes when only the side flips", () => {
		expect(annotationsVersion([annotation(5, "additions")])).not.toBe(
			annotationsVersion([annotation(5, "deletions")]),
		);
	});

	it("is stable for identical annotation content", () => {
		expect(annotationsVersion([annotation(7, "deletions")])).toBe(
			annotationsVersion([annotation(7, "deletions")]),
		);
	});

	it("differs from the empty state for any annotation", () => {
		expect(annotationsVersion([annotation(0)])).not.toBe(0);
	});
});
