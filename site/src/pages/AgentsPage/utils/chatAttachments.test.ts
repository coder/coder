import { describe, expect, it } from "vitest";
import {
	isChatAttachmentFile,
	isStrictChatAttachmentFile,
	renameChatFileForUpload,
	sanitizeChatFileName,
	workspaceFileReferencePart,
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

describe("isStrictChatAttachmentFile", () => {
	it("accepts allowlisted MIME types", () => {
		const file = new File(["png"], "image.png", { type: "image/png" });

		expect(isStrictChatAttachmentFile(file)).toBe(true);
	});

	it("rejects files with an empty MIME type", () => {
		const file = new File(["markdown"], "notes.md");

		expect(isStrictChatAttachmentFile(file)).toBe(false);
	});

	it("rejects application/octet-stream files", () => {
		const file = new File(["unknown"], "attachment.bin", {
			type: "application/octet-stream",
		});

		expect(isStrictChatAttachmentFile(file)).toBe(false);
	});

	it("rejects unsupported MIME types", () => {
		const file = new File(["zip"], "archive.zip", {
			type: "application/zip",
		});

		expect(isStrictChatAttachmentFile(file)).toBe(false);
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

describe("workspaceFileReferencePart", () => {
	it("defaults missing media type to application/octet-stream", () => {
		expect(
			workspaceFileReferencePart({
				path: "/home/coder/.coder/chats/chat-1/files/data.bin",
				name: "data.bin",
				size: 12,
				mediaType: "",
			}),
		).toMatchObject({
			workspace_file_media_type: "application/octet-stream",
		});
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
