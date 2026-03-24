import { describe, expect, it } from "vitest";
import { countInvisibleCharacters } from "./invisibleUnicode";

describe("countInvisibleCharacters", () => {
	it("returns 0 for normal text", () => {
		expect(countInvisibleCharacters("Hello, world!")).toBe(0);
		expect(
			countInvisibleCharacters("Regular ASCII text with punctuation."),
		).toBe(0);
		expect(countInvisibleCharacters("日本語テキスト")).toBe(0);
		expect(countInvisibleCharacters("👋🏽 emoji are fine")).toBe(0);
	});

	it("returns 0 for empty string", () => {
		expect(countInvisibleCharacters("")).toBe(0);
	});

	it("counts ZWS characters correctly", () => {
		expect(countInvisibleCharacters("test\u200b\u200b\u200btext")).toBe(3);
		expect(countInvisibleCharacters("\u200b")).toBe(1);
	});

	it("counts mixed invisible characters", () => {
		// ZWS + soft hyphen + bidi LRE + BOM = 4 invisible chars.
		const text = "a\u200b\u00adb\u202a\ufeffc";
		expect(countInvisibleCharacters(text)).toBe(4);
	});

	it("handles the steganography pattern", () => {
		// ZWS start + ZWNJ/invisible-separator binary + ZWJ end.
		// This is a common zero-width steganography encoding scheme.
		const payload = "\u200b\u200c\u2063\u200c\u2063\u200d";
		expect(countInvisibleCharacters(payload)).toBe(6);
	});

	it("handles text with interleaved ZWS", () => {
		// "h​e​l​l​o" — 4 ZWS between visible chars.
		expect(countInvisibleCharacters("h\u200be\u200bl\u200bl\u200bo")).toBe(4);
	});

	it("does NOT count tag characters", () => {
		// Tag characters U+E0001–U+E007F are used in subdivision flag
		// emoji (e.g. 🏴󠁧󠁢󠁥󠁮󠁧󠁿) and are deliberately excluded from the
		// strip list. They appear as surrogate pairs in UTF-16.
		const text =
			"text\u{E0001}\u{E0067}\u{E0062}\u{E0065}\u{E006E}\u{E0067}\u{E007F}more";
		expect(countInvisibleCharacters(text)).toBe(0);
	});

	it("counts all bidi override codepoints", () => {
		// U+202A through U+202E (5 codepoints).
		const bidi = "\u202a\u202b\u202c\u202d\u202e";
		expect(countInvisibleCharacters(bidi)).toBe(5);
	});

	it("counts all bidi isolate codepoints", () => {
		// U+2066 through U+2069 (4 codepoints).
		const isolates = "\u2066\u2067\u2068\u2069";
		expect(countInvisibleCharacters(isolates)).toBe(4);
	});

	it("counts invisible operator codepoints", () => {
		// U+2060 through U+2064 (5 codepoints).
		const operators = "\u2060\u2061\u2062\u2063\u2064";
		expect(countInvisibleCharacters(operators)).toBe(5);
	});

	it("counts LTR/RTL marks", () => {
		expect(countInvisibleCharacters("foo\u200ebar")).toBe(1);
		expect(countInvisibleCharacters("foo\u200fbar")).toBe(1);
	});
});
