import { describe, expect, it } from "vitest";
import { clampContent, exceedsClampThreshold } from "./text";

describe("clampContent", () => {
	it("returns the input unchanged when shorter than the limit", () => {
		expect(clampContent("hello", 10)).toBe("hello");
	});

	it("trims whitespace before measuring length", () => {
		expect(clampContent("  hi  ", 5)).toBe("hi");
	});

	it("truncates at code-point boundaries so surrogate pairs stay intact", () => {
		// 79 one-code-point chars + one two-UTF-16-unit emoji = 80 code
		// points (and 81 UTF-16 code units). A plain String.slice at 80
		// would split the surrogate pair; Array.from keeps it whole.
		const input = `${"a".repeat(79)}🎉extra`;
		const clamped = clampContent(input, 80);
		expect(clamped.endsWith("🎉…")).toBe(true);
		// No stray lone surrogates should remain in the output.
		for (let i = 0; i < clamped.length; i++) {
			const code = clamped.charCodeAt(i);
			if (code >= 0xd800 && code <= 0xdbff) {
				// high surrogate must be followed by a low surrogate
				const next = clamped.charCodeAt(i + 1);
				expect(next >= 0xdc00 && next <= 0xdfff).toBe(true);
			}
		}
	});

	it("appends an ellipsis when truncating", () => {
		expect(clampContent("abcdefghij", 4)).toBe("abcd…");
	});

	it("returns an empty string for whitespace-only input", () => {
		expect(clampContent("   ", 10)).toBe("");
	});

	it("keeps text exactly at the limit unchanged", () => {
		expect(clampContent("abcde", 5)).toBe("abcde");
	});

	it("strips trailing whitespace before appending the ellipsis", () => {
		expect(clampContent("abc     defghij", 6)).toBe("abc…");
	});
});

describe("exceedsClampThreshold", () => {
	it("returns false when the trimmed content fits", () => {
		expect(exceedsClampThreshold("  short  ", 10)).toBe(false);
	});

	it("returns false when UTF-16 length exceeds but code-point count fits", () => {
		// A string of 80 emoji is 160 UTF-16 units but 80 code points.
		const emoji = "🎉".repeat(80);
		expect(exceedsClampThreshold(emoji, 80)).toBe(false);
	});

	it("returns true when code-point count exceeds the limit", () => {
		expect(exceedsClampThreshold("🎉".repeat(81), 80)).toBe(true);
	});
});
