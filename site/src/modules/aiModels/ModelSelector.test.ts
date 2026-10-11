import { describe, expect, it } from "vitest";
import { formatContextLimit } from "./ModelSelector";

describe("formatContextLimit", () => {
	it.each([
		{ tokens: 800, expected: "800" },
		{ tokens: 999.6, expected: "1K" },
		{ tokens: 128_000, expected: "128K" },
		{ tokens: 999_999, expected: "1.0M" },
		{ tokens: 1_000_000, expected: "1M" },
		{ tokens: 1_500_000, expected: "1.5M" },
	])("formats $tokens tokens as $expected", ({ tokens, expected }) => {
		expect(formatContextLimit(tokens)).toBe(expected);
	});
});
