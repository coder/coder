import { beforeEach, describe, expect, it } from "vitest";
import {
	chatDraftAttachmentStorageKey,
	clearChatDraftAttachmentRecords,
	fileToDataURL,
	removeChatDraftAttachmentRecord,
	restoreChatDraftAttachments,
	upsertChatDraftAttachmentRecord,
} from "./chatDraftAttachmentStorage";
import { readAgentAttachmentText } from "./fileAttachmentLimits";

const organizationId = "org-1";
const chatId = "chat-1";
const storageKey = chatDraftAttachmentStorageKey(organizationId, chatId);

const readStoredRecords = () =>
	JSON.parse(localStorage.getItem(storageKey) ?? "[]");

describe("chatDraftAttachmentStorage", () => {
	beforeEach(() => {
		localStorage.clear();
	});

	it("restores persisted pending attachments from data URLs", async () => {
		const file = new File(["hello"], "note.txt", {
			type: "text/plain",
			lastModified: 1,
		});

		const result = upsertChatDraftAttachmentRecord({
			status: "pending",
			clientId: "client-1",
			fileName: file.name,
			fileType: file.type,
			lastModified: file.lastModified,
			size: file.size,
			organizationId,
			chatId,
			payload: await fileToDataURL(file),
		});

		expect(result).toEqual({ ok: true });
		const restored = restoreChatDraftAttachments(organizationId, chatId);
		expect(restored).toHaveLength(1);
		expect(restored[0].file.name).toBe("note.txt");
		expect(restored[0].file.type).toBe("text/plain");
		expect(await readAgentAttachmentText(restored[0].file)).toBe("hello");
	});

	it("prunes malformed records and data URLs that do not match metadata", () => {
		localStorage.setItem(
			storageKey,
			JSON.stringify([
				{
					status: "uploaded",
					clientId: "good",
					fileId: "file-good",
					fileName: "good.png",
					fileType: "image/png",
					lastModified: 2,
					size: 10,
					organizationId,
					chatId,
				},
				{
					status: "pending",
					clientId: "wrong-media-type",
					fileName: "bad.txt",
					fileType: "text/plain",
					lastModified: 3,
					size: 10,
					organizationId,
					chatId,
					payload: "data:text/html;base64,PGgxPmJhZDwvaDE+",
				},
				{
					status: "pending",
					clientId: "bad-base64",
					fileName: "bad.txt",
					fileType: "text/plain",
					lastModified: 4,
					size: 10,
					organizationId,
					chatId,
					payload: "data:text/plain;base64,not valid base64!",
				},
				{ status: "uploaded", clientId: "partial" },
			]),
		);

		const restored = restoreChatDraftAttachments(organizationId, chatId);

		expect(restored).toHaveLength(1);
		expect(restored[0].record.clientId).toBe("good");
		expect(readStoredRecords()).toEqual([
			expect.objectContaining({ clientId: "good" }),
		]);
	});

	it("deduplicates records by clientId and uploaded fileId", () => {
		upsertChatDraftAttachmentRecord({
			status: "uploaded",
			clientId: "client-a",
			fileId: "file-1",
			fileName: "first.png",
			fileType: "image/png",
			lastModified: 5,
			size: 10,
			organizationId,
			chatId,
		});
		upsertChatDraftAttachmentRecord({
			status: "uploaded",
			clientId: "client-b",
			fileId: "file-1",
			fileName: "duplicate.png",
			fileType: "image/png",
			lastModified: 6,
			size: 10,
			organizationId,
			chatId,
		});
		upsertChatDraftAttachmentRecord({
			status: "uploaded",
			clientId: "client-a",
			fileId: "file-2",
			fileName: "updated.png",
			fileType: "image/png",
			lastModified: 7,
			size: 10,
			organizationId,
			chatId,
		});

		const records = readStoredRecords();
		expect(records).toHaveLength(2);
		expect(records).toEqual(
			expect.arrayContaining([
				expect.objectContaining({ clientId: "client-a", fileId: "file-2" }),
				expect.objectContaining({ clientId: "client-b", fileId: "file-1" }),
			]),
		);
	});

	it("prunes expired draft records from older chat keys", () => {
		const oldKey = chatDraftAttachmentStorageKey(organizationId, "old-chat");
		localStorage.setItem(
			oldKey,
			JSON.stringify([
				{
					status: "uploaded",
					clientId: "expired",
					fileId: "file-expired",
					fileName: "expired.png",
					fileType: "image/png",
					lastModified: 10,
					size: 10,
					updatedAt: Date.now() - 31 * 24 * 60 * 60 * 1000,
					organizationId,
					chatId: "old-chat",
				},
			]),
		);

		restoreChatDraftAttachments(organizationId, chatId);

		expect(localStorage.getItem(oldKey)).toBeNull();
	});

	it("removes individual records and clears a chat scope", () => {
		upsertChatDraftAttachmentRecord({
			status: "uploaded",
			clientId: "client-a",
			fileId: "file-a",
			fileName: "a.png",
			fileType: "image/png",
			lastModified: 8,
			size: 10,
			organizationId,
			chatId,
		});
		upsertChatDraftAttachmentRecord({
			status: "uploaded",
			clientId: "client-b",
			fileId: "file-b",
			fileName: "b.png",
			fileType: "image/png",
			lastModified: 9,
			size: 10,
			organizationId,
			chatId,
		});

		expect(
			removeChatDraftAttachmentRecord(organizationId, chatId, "client-a"),
		).toEqual({ ok: true });
		expect(readStoredRecords()).toEqual([
			expect.objectContaining({ clientId: "client-b" }),
		]);
		expect(clearChatDraftAttachmentRecords(organizationId, chatId)).toEqual({
			ok: true,
		});
		expect(localStorage.getItem(storageKey)).toBeNull();
	});
});
