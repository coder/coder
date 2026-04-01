import {
	decodeInlineTextAttachment,
	formatTextAttachmentPreview,
} from "./fetchTextAttachment";

const encodeUtf8Base64 = (value: string) => {
	const bytes = new TextEncoder().encode(value);
	return btoa(String.fromCharCode(...bytes));
};

describe("formatTextAttachmentPreview", () => {
	it('returns "Pasted text" for empty content', () => {
		expect(formatTextAttachmentPreview("")).toBe("Pasted text");
		expect(formatTextAttachmentPreview("   \n\t ")).toBe("Pasted text");
	});

	it("truncates longer text to the requested limit", () => {
		expect(formatTextAttachmentPreview("abcdefgh", 5)).toBe("abcde");
	});

	it("normalizes whitespace before building the preview", () => {
		expect(
			formatTextAttachmentPreview("  hello\n\nworld\tfrom\t tests  "),
		).toBe("hello world from tests");
	});

	it("preserves whole unicode code points when truncating emoji", () => {
		expect(formatTextAttachmentPreview("🙂🙂🙂", 2)).toBe("🙂🙂");
	});

	it("returns text shorter than the limit unchanged", () => {
		expect(formatTextAttachmentPreview("short text", 20)).toBe("short text");
	});
});

describe("decodeInlineTextAttachment", () => {
	afterEach(() => {
		vi.restoreAllMocks();
	});

	it("decodes base64-encoded UTF-8 text", () => {
		const text = "Hello 👋 café";

		expect(decodeInlineTextAttachment(encodeUtf8Base64(text))).toBe(text);
	});

	it("falls back to the raw string when base64 decoding fails", () => {
		const warn = vi.spyOn(console, "warn").mockImplementation(() => undefined);
		const raw = "not-base64!";

		expect(decodeInlineTextAttachment(raw)).toBe(raw);
		expect(warn).toHaveBeenCalled();
	});
});
