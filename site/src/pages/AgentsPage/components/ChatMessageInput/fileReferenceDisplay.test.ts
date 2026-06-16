import { describe, expect, it } from "vitest";
import {
	getFileReferenceDisplay,
	hasInlineContentAfter,
	hasInlineContentBefore,
} from "./fileReferenceDisplay";

describe("getFileReferenceDisplay", () => {
	it("returns the basename and line range title", () => {
		expect(
			getFileReferenceDisplay({
				fileName: "site/src/pages/AgentsPage/components/Button.tsx",
				startLine: 12,
				endLine: 18,
			}),
		).toEqual({
			shortFile: "Button.tsx",
			lineRange: "L12-L18",
			title: "site/src/pages/AgentsPage/components/Button.tsx:L12-L18",
		});
	});

	it("keeps the raw filename for paths without separators", () => {
		expect(
			getFileReferenceDisplay({
				fileName: "main.go",
				startLine: 7,
				endLine: 7,
			}),
		).toEqual({
			shortFile: "main.go",
			lineRange: "L7",
			title: "main.go:L7",
		});
	});
});

describe("inline spacing helpers", () => {
	const parts = [
		{ type: "text", text: "prefix" },
		{ type: "file-reference" },
		{ type: "text", text: "suffix" },
	] as const;

	it("treats abutting text as inline content", () => {
		expect(hasInlineContentBefore(parts, 1)).toBe(true);
		expect(hasInlineContentAfter(parts, 1)).toBe(true);
	});

	it("ignores empty and whitespace-only text parts", () => {
		const whitespaceParts = [
			{ type: "text", text: "" },
			{ type: "text", text: "  " },
			{ type: "file-reference" },
			{ type: "text", text: "\n" },
		] as const;

		expect(hasInlineContentBefore(whitespaceParts, 2)).toBe(false);
		expect(hasInlineContentAfter(whitespaceParts, 2)).toBe(false);
	});

	it("treats adjacent file references as inline neighbors", () => {
		const adjacentReferences = [
			{ type: "file-reference" },
			{ type: "file-reference" },
			{ type: "file-reference" },
		] as const;

		expect(hasInlineContentBefore(adjacentReferences, 1)).toBe(true);
		expect(hasInlineContentAfter(adjacentReferences, 1)).toBe(true);
	});

	it("uses the first non-empty text part on each side", () => {
		const sparseParts = [
			{ type: "text", text: "" },
			{ type: "text", text: "leading" },
			{ type: "file-reference" },
			{ type: "text", text: "" },
			{ type: "text", text: "trailing" },
		] as const;

		expect(hasInlineContentBefore(sparseParts, 2)).toBe(true);
		expect(hasInlineContentAfter(sparseParts, 2)).toBe(true);
	});
});
