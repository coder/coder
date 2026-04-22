import {
	decodeInlineTextAttachment,
	encodeInlineTextAttachment,
	fetchTextAttachmentContent,
	formatTextAttachmentPreview,
	getTextAttachmentErrorMessage,
} from "./fetchTextAttachment";

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

		expect(decodeInlineTextAttachment(encodeInlineTextAttachment(text))).toBe(
			text,
		);
	});

	it("falls back to the raw string when base64 decoding fails", () => {
		const warn = vi.spyOn(console, "warn").mockImplementation(() => undefined);
		const raw = "not-base64!";

		expect(decodeInlineTextAttachment(raw)).toBe(raw);
		expect(warn).toHaveBeenCalled();
	});
});

describe("fetchTextAttachmentContent", () => {
	afterEach(() => {
		vi.restoreAllMocks();
	});

	it("returns a loaded result when the fetch succeeds", async () => {
		vi.spyOn(globalThis, "fetch").mockResolvedValue(
			new Response("hello from the server", { status: 200 }),
		);

		const fileId = "folder/file-1?preview=yes";
		await expect(fetchTextAttachmentContent(fileId)).resolves.toEqual({
			kind: "loaded",
			content: "hello from the server",
		});
		expect(globalThis.fetch).toHaveBeenCalledWith(
			"/api/experimental/chats/files/folder%2Ffile-1%3Fpreview%3Dyes",
			expect.anything(),
		);
	});

	it("returns the API message for unauthorized attachment fetches", async () => {
		vi.spyOn(globalThis, "fetch").mockResolvedValue(
			new Response(
				JSON.stringify({ message: "Sign in again to view files." }),
				{
					status: 401,
					statusText: "Unauthorized",
					headers: { "Content-Type": "application/json" },
				},
			),
		);

		await expect(fetchTextAttachmentContent("file-2")).resolves.toEqual({
			kind: "failed",
			detail: "Sign in again to view files.",
		});
	});

	it("returns a classified failure when the fetch responds non-OK", async () => {
		vi.spyOn(globalThis, "fetch").mockResolvedValue(
			new Response("nope", { status: 503 }),
		);

		const result = await fetchTextAttachmentContent("file-3");
		expect(result.kind).not.toBe("loaded");
	});
});

describe("getTextAttachmentErrorMessage", () => {
	it("suppresses DOMException abort errors", () => {
		expect(
			getTextAttachmentErrorMessage(new DOMException("aborted", "AbortError")),
		).toBeNull();
	});

	it("suppresses structural abort errors", () => {
		expect(getTextAttachmentErrorMessage({ name: "AbortError" })).toBeNull();
	});

	it("falls back to the retry message for other failures", () => {
		expect(getTextAttachmentErrorMessage(new Error("boom"))).toBe(
			"Couldn't load preview. Select again to retry.",
		);
	});
});
