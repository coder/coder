import { act, renderHook } from "@testing-library/react";
import { afterEach, beforeEach, describe, expect, it, vi } from "vitest";
import { API } from "#/api/api";
import { chatDraftAttachmentStorageKey } from "../utils/chatDraftAttachmentStorage";
import {
	resetChatDraftAttachmentRegistryForTest,
	useChatDraftAttachments,
} from "./useChatDraftAttachments";

type Deferred<T> = {
	promise: Promise<T>;
	resolve: (value: T | PromiseLike<T>) => void;
	reject: (reason?: unknown) => void;
};

const createDeferred = <T>(): Deferred<T> => {
	let resolve!: (value: T | PromiseLike<T>) => void;
	let reject!: (reason?: unknown) => void;
	const promise = new Promise<T>((res, rej) => {
		resolve = res;
		reject = rej;
	});
	return { promise, resolve, reject };
};

const orgID = "org-1";
const chatID = "chat-a";
const storageKey = chatDraftAttachmentStorageKey(orgID, chatID);

const parseStoredDrafts = () =>
	JSON.parse(localStorage.getItem(storageKey) ?? "[]");

describe("useChatDraftAttachments", () => {
	let originalCreateObjectURL: typeof URL.createObjectURL | undefined;
	let originalRevokeObjectURL: typeof URL.revokeObjectURL | undefined;

	beforeEach(() => {
		localStorage.clear();
		resetChatDraftAttachmentRegistryForTest();
		originalCreateObjectURL = URL.createObjectURL;
		originalRevokeObjectURL = URL.revokeObjectURL;
		Object.defineProperty(URL, "createObjectURL", {
			configurable: true,
			value: vi.fn(() => "blob:attachment-preview"),
		});
		Object.defineProperty(URL, "revokeObjectURL", {
			configurable: true,
			value: vi.fn(),
		});
		vi.spyOn(globalThis, "fetch").mockResolvedValue(new Response());
	});

	afterEach(() => {
		resetChatDraftAttachmentRegistryForTest();
		localStorage.clear();
		vi.restoreAllMocks();
		Object.defineProperty(URL, "createObjectURL", {
			configurable: true,
			value: originalCreateObjectURL,
		});
		Object.defineProperty(URL, "revokeObjectURL", {
			configurable: true,
			value: originalRevokeObjectURL,
		});
	});

	it("restores chat draft attachments by organization and chat without duplicating active uploads", async () => {
		const upload = createDeferred<{ id: string }>();
		const uploadSpy = vi
			.spyOn(API.experimental, "uploadChatFile")
			.mockReturnValue(upload.promise);
		const { result, unmount } = renderHook(() =>
			useChatDraftAttachments(orgID, chatID),
		);
		const file = new File(["hello"], "note.txt", {
			type: "text/plain",
			lastModified: 1,
		});

		act(() => {
			result.current.handleAttach([file]);
		});

		await vi.waitFor(() => {
			const stored = parseStoredDrafts();
			expect(stored).toHaveLength(1);
			expect(stored[0]).toMatchObject({
				status: "uploading",
				fileName: "note.txt",
				organizationId: orgID,
				chatId: chatID,
			});
			expect(stored[0].payload).toEqual(expect.any(String));
		});
		expect(uploadSpy).toHaveBeenCalledTimes(1);
		unmount();

		const otherChat = renderHook(() =>
			useChatDraftAttachments(orgID, "chat-b"),
		);
		expect(otherChat.result.current.attachments).toHaveLength(0);
		otherChat.unmount();

		const restored = renderHook(() => useChatDraftAttachments(orgID, chatID));
		expect(restored.result.current.attachments).toHaveLength(1);
		expect(restored.result.current.attachments[0].name).toBe("note.txt");
		expect(uploadSpy).toHaveBeenCalledTimes(1);

		await act(async () => {
			upload.resolve({ id: "file-1" });
		});
		await vi.waitFor(() => {
			const state = restored.result.current.uploadStates.get(
				restored.result.current.attachments[0],
			);
			expect(state).toMatchObject({ status: "uploaded", fileId: "file-1" });
		});
		const stored = parseStoredDrafts();
		expect(stored).toHaveLength(1);
		expect(stored[0]).toMatchObject({ status: "uploaded", fileId: "file-1" });
		expect(stored[0].payload).toBeUndefined();
		restored.unmount();
	});

	it("does not resurrect removed in-flight attachments after upload completion", async () => {
		const upload = createDeferred<{ id: string }>();
		vi.spyOn(API.experimental, "uploadChatFile").mockReturnValue(
			upload.promise,
		);
		const { result, unmount } = renderHook(() =>
			useChatDraftAttachments(orgID, chatID),
		);
		const file = new File(["hello"], "photo.png", {
			type: "image/png",
			lastModified: 2,
		});

		act(() => {
			result.current.handleAttach([file]);
		});
		await vi.waitFor(() => {
			expect(result.current.attachments).toHaveLength(1);
		});

		act(() => {
			result.current.handleRemoveAttachment(file);
		});
		expect(result.current.attachments).toHaveLength(0);
		expect(localStorage.getItem(storageKey)).toBeNull();

		await act(async () => {
			upload.resolve({ id: "file-removed" });
		});
		expect(result.current.attachments).toHaveLength(0);
		expect(localStorage.getItem(storageKey)).toBeNull();
		unmount();
	});

	it("keeps quota-limited attachments in memory and clears the warning after metadata persists", async () => {
		const upload = createDeferred<{ id: string }>();
		vi.spyOn(API.experimental, "uploadChatFile").mockReturnValue(
			upload.promise,
		);
		const realSetItem = Storage.prototype.setItem;
		vi.spyOn(Storage.prototype, "setItem").mockImplementation(function (
			this: Storage,
			key: string,
			value: string,
		) {
			if (key === storageKey && String(value).includes('"payload"')) {
				throw new DOMException("Quota exceeded", "QuotaExceededError");
			}
			return realSetItem.call(this, key, value);
		});
		const { result, unmount } = renderHook(() =>
			useChatDraftAttachments(orgID, chatID),
		);
		const file = new File(["hello"], "large.txt", {
			type: "text/plain",
			lastModified: 3,
		});

		act(() => {
			result.current.handleAttach([file]);
		});

		await vi.waitFor(() => {
			const state = result.current.uploadStates.get(file);
			expect(state?.draftWarning).toContain("too large to save as a draft");
		});
		expect(localStorage.getItem(storageKey)).toBeNull();

		await act(async () => {
			upload.resolve({ id: "file-lightweight" });
		});
		await vi.waitFor(() => {
			const state = result.current.uploadStates.get(file);
			expect(state).toMatchObject({
				status: "uploaded",
				fileId: "file-lightweight",
			});
			expect(state?.draftWarning).toBeUndefined();
		});
		const stored = parseStoredDrafts();
		expect(stored).toHaveLength(1);
		expect(stored[0]).toMatchObject({
			status: "uploaded",
			fileId: "file-lightweight",
		});
		expect(stored[0].payload).toBeUndefined();
		unmount();
	});

	it("prunes corrupt and wrong-scope stored records during restore", () => {
		localStorage.setItem(
			storageKey,
			JSON.stringify([
				{
					status: "uploaded",
					clientId: "good",
					fileId: "file-good",
					fileName: "good.png",
					fileType: "image/png",
					lastModified: 4,
					size: 10,
					organizationId: orgID,
					chatId: chatID,
				},
				{
					status: "uploaded",
					clientId: "other-org",
					fileId: "file-other",
					fileName: "other.png",
					fileType: "image/png",
					lastModified: 4,
					size: 10,
					organizationId: "org-2",
					chatId: chatID,
				},
				{ status: "uploaded", clientId: "bad" },
			]),
		);

		const { result, unmount } = renderHook(() =>
			useChatDraftAttachments(orgID, chatID),
		);

		expect(result.current.attachments).toHaveLength(1);
		expect(result.current.attachments[0].name).toBe("good.png");
		const stored = parseStoredDrafts();
		expect(stored).toHaveLength(1);
		expect(stored[0].clientId).toBe("good");
		unmount();
	});
});
