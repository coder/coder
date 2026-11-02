import { describe, expect, it } from "vitest";
import { Osc52Parser } from "./osc52";

const encoder = new TextEncoder();

describe("Osc52Parser", () => {
	it("decodes a sequence split across chunks", () => {
		const parser = new Osc52Parser();
		const sequence = encoder.encode(`\x1b]52;c;${btoa("Coder is Cool")}\x07`);

		const results = [
			...parser.feed(sequence.slice(0, 3)),
			...parser.feed(sequence.slice(3, 9)),
			...parser.feed(sequence.slice(9)),
		];

		expect(results).toEqual(["Coder is Cool"]);
	});

	it("accepts the ESC backslash terminator and UTF-8 text", () => {
		const parser = new Osc52Parser();
		const base64 = btoa(String.fromCharCode(...encoder.encode("héllo ✓")));

		expect(parser.feed(`prefix\x1b]52;c;${base64}\x1b\\suffix`)).toEqual([
			"héllo ✓",
		]);
	});

	it("ignores other escape sequences and aborted sequences", () => {
		const parser = new Osc52Parser();
		const base64 = btoa("kept");

		expect(
			parser.feed(
				`\x1b[31mred\x1b]0;title\x07\x1b]52;c;${btoa("dropped")}\x1b[0m\x1b]52;c;${base64}\x07`,
			),
		).toEqual(["kept"]);
	});
});
