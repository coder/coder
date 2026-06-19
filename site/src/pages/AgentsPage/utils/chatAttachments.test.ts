import { describe, expect, it } from "vitest";
import {
	attachmentToContentPart,
	formatInlinedAttachmentText,
	isChatAttachmentFile,
	isInlinableTextAttachment,
	renameChatFileForUpload,
	sanitizeChatFileName,
} from "./chatAttachments";

describe("isChatAttachmentFile", () => {
	it("accepts allowlisted MIME types", () => {
		const file = new File(["png"], "image.png", { type: "image/png" });

		expect(isChatAttachmentFile(file)).toBe(true);
	});

	it("accepts files with an empty MIME type", () => {
		const file = new File(["markdown"], "notes.md");

		expect(isChatAttachmentFile(file)).toBe(true);
	});

	it("accepts application/octet-stream files", () => {
		const file = new File(["unknown"], "attachment.bin", {
			type: "application/octet-stream",
		});

		expect(isChatAttachmentFile(file)).toBe(true);
	});

	it("rejects unsupported MIME types", () => {
		const file = new File(["zip"], "archive.zip", {
			type: "application/zip",
		});

		expect(isChatAttachmentFile(file)).toBe(false);
	});
});

describe("isInlinableTextAttachment", () => {
	it.each([
		["application/json", "data.json", true],
		["text/plain", "notes.txt", true],
		["text/markdown", "readme.md", true],
		["text/csv", "rows.csv", true],
		["image/png", "image.png", false],
		["application/pdf", "doc.pdf", false],
	])("classifies %s by declared type", (type, name, expected) => {
		const file = new File(["x"], name, { type });

		expect(isInlinableTextAttachment(file)).toBe(expected);
	});

	it.each([
		["data.json", true],
		["rows.csv", true],
		["readme.md", true],
		["readme.markdown", true],
		["notes.txt", true],
		["image.png", false],
		["archive.zip", false],
	])("falls back to the extension when type is unknown: %s", (name, expected) => {
		const emptyType = new File(["x"], name);
		const octetStream = new File(["x"], name, {
			type: "application/octet-stream",
		});

		expect(isInlinableTextAttachment(emptyType)).toBe(expected);
		expect(isInlinableTextAttachment(octetStream)).toBe(expected);
	});
});

describe("formatInlinedAttachmentText", () => {
	it("labels the content with the filename", () => {
		expect(formatInlinedAttachmentText("data.json", '{"a":1}')).toBe(
			'Attached file: data.json\n\n{"a":1}',
		);
	});
});

describe("attachmentToContentPart", () => {
	it("emits a text part when inlined content is present", () => {
		expect(
			attachmentToContentPart({ fileId: "f1", textContent: "hello" }),
		).toEqual({ type: "text", text: "hello" });
	});

	it("emits a file part when no inlined content is present", () => {
		expect(attachmentToContentPart({ fileId: "f1" })).toEqual({
			type: "file",
			file_id: "f1",
		});
	});
});

describe("sanitizeChatFileName", () => {
	it.each([
		// Already safe.
		["clean.pdf", "clean.pdf"],
		// Spaces, parens collapsed into a single underscore each.
		["My Report (final).pdf", "My_Report_final_.pdf"],
		// `!` is kept; only `&` and the space become underscores.
		["weird & stuff!.txt", "weird_stuff!.txt"],
		// Path separators (forward and backslash) become underscores.
		["path/with\\slash.png", "path_with_slash.png"],
		// Leading dots/spaces/underscores are trimmed.
		["   .leading.dots.txt", "leading.dots.txt"],
		// Non-ASCII letters survive.
		["日本語のファイル.txt", "日本語のファイル.txt"],
		// Emoji survive.
		["🔥emoji🔥.png", "🔥emoji🔥.png"],
		// Control characters are stripped (replaced and trimmed).
		["\u0000\u0001\tcontrol.bin", "control.bin"],
		// Underscore-only collapses to empty then falls back to "file".
		["___", "file"],
		// Empty input falls back to "file".
		["", "file"],
		// Trailing problem characters are also trimmed.
		["foo!.pdf ", "foo!.pdf"],
	])("sanitizes %j to %j", (input, expected) => {
		expect(sanitizeChatFileName(input)).toBe(expected);
	});
});

describe("renameChatFileForUpload", () => {
	it("returns the same File reference when the name is already safe", () => {
		const file = new File(["png"], "clean.png", { type: "image/png" });

		// Identity matters: useFileAttachments keys preview-URL,
		// upload-state, and text-content Maps on the File object.
		expect(renameChatFileForUpload(file)).toBe(file);
	});

	it("returns a new File with a sanitized name when needed", () => {
		const file = new File(["pdf-bytes"], "My Report (final).pdf", {
			type: "application/pdf",
			lastModified: 1_700_000_000_000,
		});

		const renamed = renameChatFileForUpload(file);

		expect(renamed).not.toBe(file);
		expect(renamed.name).toBe("My_Report_final_.pdf");
		expect(renamed.type).toBe("application/pdf");
		expect(renamed.lastModified).toBe(1_700_000_000_000);
		// File size preserved; byte content is covered transitively by
		// the File constructor, and jsdom's Blob backing in this
		// project is not reliable enough for an explicit text() probe.
		expect(renamed.size).toBe(file.size);
	});
});
