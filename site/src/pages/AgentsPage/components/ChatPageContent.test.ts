import { describe, expect, it } from "vitest";
import type { WorkspaceUploadState } from "../hooks/useWorkspaceFileUploads";
import type { PendingWorkspaceUpload } from "../utils/chatAttachments";
import type { UploadState } from "./AttachmentPreview";
import {
	collectUploadedAttachments,
	collectUploadedWorkspaceFiles,
	collectWorkspaceUploadsForSend,
} from "./ChatPageContent";

describe("collectUploadedAttachments", () => {
	it("returns uploaded attachments and counts failed uploads", () => {
		const uploaded = new File(["ok"], "image.png", { type: "image/png" });
		const failed = new File(["bad"], "broken.png", { type: "image/png" });
		const pending = new File(["wait"], "pending.png", { type: "image/png" });
		const states = new Map<File, UploadState>([
			[uploaded, { status: "uploaded", fileId: "file-1" }],
			[failed, { status: "error", error: "boom" }],
			[pending, { status: "uploading" }],
		]);

		expect(
			collectUploadedAttachments([uploaded, failed, pending], states),
		).toEqual({
			attachments: [{ fileId: "file-1", mediaType: "image/png" }],
			skippedErrors: 1,
		});
	});
});

describe("collectUploadedWorkspaceFiles", () => {
	it("returns uploaded workspace files and counts failed uploads", () => {
		const uploaded = new File(["ok"], "data.csv", { type: "text/csv" });
		const failed = new File(["bad"], "broken.csv", { type: "text/csv" });
		const states = new Map<File, WorkspaceUploadState>([
			[
				uploaded,
				{
					status: "uploaded",
					path: "/home/coder/.coder/chats/chat-1/files/data.csv",
					name: "data.csv",
					size: 128,
					mediaType: "text/csv",
				},
			],
			[failed, { status: "error", error: "boom" }],
		]);

		expect(collectUploadedWorkspaceFiles([uploaded, failed], states)).toEqual({
			uploads: [
				{
					path: "/home/coder/.coder/chats/chat-1/files/data.csv",
					name: "data.csv",
					size: 128,
					mediaType: "text/csv",
				},
			],
			skippedErrors: 1,
		});
	});
});

describe("collectWorkspaceUploadsForSend", () => {
	it("keeps edited workspace references and appends new uploads", () => {
		const edited: PendingWorkspaceUpload = {
			path: "/home/coder/.coder/chats/chat-1/files/old.csv",
			name: "old.csv",
			size: 64,
			mediaType: "text/csv",
		};
		const uploaded = new File(["ok"], "new.csv", { type: "text/csv" });
		const states = new Map<File, WorkspaceUploadState>([
			[
				uploaded,
				{
					status: "uploaded",
					path: "/home/coder/.coder/chats/chat-1/files/new.csv",
					name: "new.csv",
					size: 32,
					mediaType: "text/csv",
				},
			],
		]);

		expect(
			collectWorkspaceUploadsForSend({
				isEditing: true,
				editingUploads: [edited],
				files: [uploaded],
				states,
			}),
		).toEqual({
			uploads: [
				edited,
				{
					path: "/home/coder/.coder/chats/chat-1/files/new.csv",
					name: "new.csv",
					size: 32,
					mediaType: "text/csv",
				},
			],
			skippedErrors: 0,
		});
	});

	it("omits edited workspace references outside edit mode", () => {
		const edited: PendingWorkspaceUpload = {
			path: "/home/coder/.coder/chats/chat-1/files/old.csv",
			name: "old.csv",
			size: 64,
			mediaType: "text/csv",
		};

		expect(
			collectWorkspaceUploadsForSend({
				isEditing: false,
				editingUploads: [edited],
				files: [],
				states: new Map(),
			}),
		).toEqual({ uploads: [], skippedErrors: 0 });
	});
});
