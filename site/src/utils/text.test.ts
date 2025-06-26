import type { Nullable } from "./nullable";
import { ellipsizeText, maskText } from "./text";

describe("ellipsizeText", () => {
	it.each([
		[undefined, 10, undefined],
		[null, 10, undefined],
		["", 10, ""],
		["Hello World", "Hello World".length, "Hello World"],
		["Hello World", "Hello...".length, "Hello..."],
	])(
		"ellipsizeText(%p, %p) returns %p",
		(
			str: Nullable<string>,
			maxLength: number | undefined,
			output: Nullable<string>,
		) => {
			expect(ellipsizeText(str, maxLength)).toBe(output);
		},
	);
});

describe("maskText", () => {
	test("masks text", () => {
		expect(maskText("hello, computer!")).toBe("\u2022".repeat(16));
	});
});
