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
		// ZWNJ (U+200C) is deliberately excluded from the strip list
		// for i18n reasons, so only 4 of the 6 chars are counted.
		const payload = "\u200b\u200c\u2063\u200c\u2063\u200d";
		expect(countInvisibleCharacters(payload)).toBe(4);
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

	it("does NOT count ZWNJ (U+200C)", () => {
		// ZWNJ is required for correct rendering of Persian, Urdu,
		// and Kurdish scripts. Excluding it has negligible security
		// impact because we already strip ZWS and ZWJ which breaks
		// the steg encoding scheme.
		expect(countInvisibleCharacters("\u200c")).toBe(0);
		expect(countInvisibleCharacters("foo\u200cbar")).toBe(0);
	});

	it("counts deprecated format characters (U+206A\u2013U+206F)", () => {
		const deprecated = "\u206a\u206b\u206c\u206d\u206e\u206f";
		expect(countInvisibleCharacters(deprecated)).toBe(6);
	});

	it("counts interlinear annotation characters (U+FFF9\u2013U+FFFB)", () => {
		const annotations = "\ufff9\ufffa\ufffb";
		expect(countInvisibleCharacters(annotations)).toBe(3);
	});

	// Canonical list \u2014 must match coderd/x/chatd/sanitize_test.go
	it("detects exactly the canonical set of invisible codepoints", () => {
		// Every codepoint the detector should flag, sorted ascending.
		const expectedCodepoints: number[] = [
			0x00ad, // Soft hyphen
			0x034f, // Combining grapheme joiner
			0x061c, // Arabic letter mark
			0x180e, // Mongolian vowel separator
			0x200b, // Zero-width space
			// NOTE: 0x200C (ZWNJ) deliberately excluded for i18n.
			0x200d, // Zero-width joiner
			0x200e, // Left-to-right mark
			0x200f, // Right-to-left mark
			0x202a, // LRE
			0x202b, // RLE
			0x202c, // PDF
			0x202d, // LRO
			0x202e, // RLO
			0x2060, // Word joiner
			0x2061, // Function application
			0x2062, // Invisible times
			0x2063, // Invisible separator
			0x2064, // Invisible plus
			0x2066, // LRI
			0x2067, // RLI
			0x2068, // FSI
			0x2069, // PDI
			0x206a, // Inhibit symmetric swapping
			0x206b, // Activate symmetric swapping
			0x206c, // Inhibit Arabic form shaping
			0x206d, // Activate Arabic form shaping
			0x206e, // National digit shapes
			0x206f, // Nominal digit shapes
			0xfeff, // BOM / zero-width no-break space
			0xfff9, // Interlinear annotation anchor
			0xfffa, // Interlinear annotation separator
			0xfffb, // Interlinear annotation terminator
		];

		// Verify each expected codepoint is detected.
		for (const cp of expectedCodepoints) {
			const char = String.fromCharCode(cp);
			expect(countInvisibleCharacters(char)).toBe(1);
		}

		// Verify a few codepoints that should NOT be detected.
		const notDetected = [
			0x0041, // 'A' \u2014 normal ASCII
			0x0020, // Space \u2014 normal whitespace
			0x200c, // ZWNJ \u2014 excluded for i18n
		];
		for (const cp of notDetected) {
			const char = String.fromCharCode(cp);
			expect(countInvisibleCharacters(char)).toBe(0);
		}

		// Tag characters (U+E0067) are astral-plane and are NOT
		// detected. They appear as surrogate pairs in UTF-16.
		expect(countInvisibleCharacters("\u{E0067}")).toBe(0);
	});
});
