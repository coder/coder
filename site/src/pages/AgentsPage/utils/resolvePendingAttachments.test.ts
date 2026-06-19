import { afterEach, describe, expect, it, vi } from "vitest";
import type { UploadState } from "../components/AttachmentPreview";
import { resolvePendingAttachments } from "./resolvePendingAttachments";

const uploaded = (fileId: string): UploadState => ({
	status: "uploaded",
	fileId,
});

afterEach(() => {
	vi.restoreAllMocks();
});

describe("resolvePendingAttachments", () => {
	it("inlines a text-family file using already-read content", async () => {
		const file = new File(['{"a":1}'], "data.json", {
			type: "application/json",
		});
		const uploadStates = new Map<File, UploadState>([[file, uploaded("f1")]]);
		const textContents = new Map<File, string>([[file, '{"a":1}']]);

		const { attachments, skippedErrors } = await resolvePendingAttachments(
			[file],
			uploadStates,
			textContents,
		);

		expect(skippedErrors).toBe(0);
		expect(attachments).toEqual([
			{
				fileId: "f1",
				mediaType: "application/json",
				textContent: 'Attached file: data.json\n\n{"a":1}',
			},
		]);
	});

	it("reads text content on demand when it was not pre-read", async () => {
		const file = new File(["hello world"], "notes.txt", {
			type: "text/plain",
		});
		const uploadStates = new Map<File, UploadState>([[file, uploaded("f2")]]);

		const { attachments } = await resolvePendingAttachments(
			[file],
			uploadStates,
			new Map(),
		);

		expect(attachments[0].textContent).toBe(
			"Attached file: notes.txt\n\nhello world",
		);
	});

	it("falls back to a file part when the on-demand read fails", async () => {
		const file = new File(["x"], "broken.json", { type: "application/json" });
		Object.defineProperty(file, "text", {
			value: () => Promise.reject(new Error("read failed")),
		});
		vi.spyOn(console, "warn").mockImplementation(() => {});
		const uploadStates = new Map<File, UploadState>([[file, uploaded("f3")]]);

		const { attachments } = await resolvePendingAttachments(
			[file],
			uploadStates,
			new Map(),
		);

		expect(attachments).toEqual([
			{ fileId: "f3", mediaType: "application/json" },
		]);
		expect(console.warn).toHaveBeenCalled();
	});

	it("sends non-text files as file parts", async () => {
		const file = new File(["png"], "image.png", { type: "image/png" });
		const uploadStates = new Map<File, UploadState>([[file, uploaded("f4")]]);

		const { attachments } = await resolvePendingAttachments(
			[file],
			uploadStates,
			new Map(),
		);

		expect(attachments).toEqual([{ fileId: "f4", mediaType: "image/png" }]);
	});

	it("counts upload failures and skips non-uploaded files", async () => {
		const failed = new File(["x"], "fail.png", { type: "image/png" });
		const pending = new File(["x"], "pending.png", { type: "image/png" });
		const ok = new File(["x"], "ok.png", { type: "image/png" });
		const uploadStates = new Map<File, UploadState>([
			[failed, { status: "error", error: "nope" }],
			[pending, { status: "uploading" }],
			[ok, uploaded("f5")],
		]);

		const { attachments, skippedErrors } = await resolvePendingAttachments(
			[failed, pending, ok],
			uploadStates,
			new Map(),
		);

		expect(skippedErrors).toBe(1);
		expect(attachments).toEqual([{ fileId: "f5", mediaType: "image/png" }]);
	});
});
