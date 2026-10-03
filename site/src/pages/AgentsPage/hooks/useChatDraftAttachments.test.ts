import { act, renderHook } from "@testing-library/react";
import { afterEach, beforeEach, describe, expect, it, vi } from "vitest";
import { API } from "#/api/api";
import { createDeferred } from "#/testHelpers/deferred";
import { chatDraftAttachmentStorageKey } from "../utils/chatDraftAttachmentStorage";
import {
	resetChatDraftAttachmentRegistryForTest,
	useChatDraftAttachments,
} from "./useChatDraftAttachments";

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
		expect(
			restored.result.current.uploadStates.get(
				restored.result.current.attachments[0],
			),
		).toMatchObject({ status: "uploading" });
		await vi.waitFor(() => {
			expect(
				restored.result.current.textContents.get(
					restored.result.current.attachments[0],
				),
			).toBe("hello");
		});
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

	it("shares an active upload across simultaneous hook instances", async () => {
		const upload = createDeferred<{ id: string }>();
		const uploadSpy = vi
			.spyOn(API.experimental, "uploadChatFile")
			.mockReturnValue(upload.promise);
		const first = renderHook(() => useChatDraftAttachments(orgID, chatID));
		const file = new File(["hello"], "shared.txt", {
			type: "text/plain",
			lastModified: 11,
		});

		act(() => {
			first.result.current.handleAttach([file]);
		});

		await vi.waitFor(() => {
			expect(parseStoredDrafts()).toHaveLength(1);
		});
		const second = renderHook(() => useChatDraftAttachments(orgID, chatID));

		await vi.waitFor(() => {
			expect(first.result.current.attachments).toHaveLength(1);
			expect(second.result.current.attachments).toHaveLength(1);
		});
		expect(uploadSpy).toHaveBeenCalledTimes(1);

		await act(async () => {
			upload.resolve({ id: "file-shared" });
		});
		await vi.waitFor(() => {
			const firstState = first.result.current.uploadStates.get(
				first.result.current.attachments[0],
			);
			const secondState = second.result.current.uploadStates.get(
				second.result.current.attachments[0],
			);
			expect(firstState).toMatchObject({
				status: "uploaded",
				fileId: "file-shared",
			});
			expect(secondState).toMatchObject({
				status: "uploaded",
				fileId: "file-shared",
			});
		});

		first.unmount();
		second.unmount();
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

	it("keeps failed uploads attached with an error state", async () => {
		const upload = createDeferred<{ id: string }>();
		vi.spyOn(API.experimental, "uploadChatFile").mockReturnValue(
			upload.promise,
		);
		const { result, unmount } = renderHook(() =>
			useChatDraftAttachments(orgID, chatID),
		);
		const file = new File(["hello"], "failed.txt", {
			type: "text/plain",
			lastModified: 12,
		});

		act(() => {
			result.current.handleAttach([file]);
		});
		await vi.waitFor(() => {
			expect(result.current.attachments).toHaveLength(1);
		});

		await act(async () => {
			upload.reject(new Error("network down"));
		});
		await vi.waitFor(() => {
			const state = result.current.uploadStates.get(file);
			expect(state).toMatchObject({ status: "error" });
			expect(state?.error).toContain("network down");
		});
		expect(result.current.attachments).toHaveLength(1);

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
			expect(state?.draftWarning).toContain("could not be saved as a draft");
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

	it("keeps uploaded attachments usable when metadata cannot be persisted", async () => {
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
			if (key === storageKey && String(value).includes("file-unpersisted")) {
				throw new DOMException("Quota exceeded", "QuotaExceededError");
			}
			return realSetItem.call(this, key, value);
		});
		const { result, unmount } = renderHook(() =>
			useChatDraftAttachments(orgID, chatID),
		);
		const file = new File(["hello"], "metadata-fails.txt", {
			type: "text/plain",
			lastModified: 13,
		});

		act(() => {
			result.current.handleAttach([file]);
		});
		await vi.waitFor(() => {
			expect(parseStoredDrafts()).toHaveLength(1);
		});

		await act(async () => {
			upload.resolve({ id: "file-unpersisted" });
		});
		await vi.waitFor(() => {
			const state = result.current.uploadStates.get(file);
			expect(state).toMatchObject({
				status: "uploaded",
				fileId: "file-unpersisted",
			});
			expect(state?.draftWarning).toContain("could not be saved as a draft");
		});
		expect(parseStoredDrafts()[0].payload).toEqual(expect.any(String));

		unmount();
	});

	it("rejects files over the attachment size limit without uploading", () => {
		const uploadSpy = vi.spyOn(API.experimental, "uploadChatFile");
		const { result, unmount } = renderHook(() =>
			useChatDraftAttachments(orgID, chatID),
		);
		const file = new File([new Uint8Array(10 * 1024 * 1024 + 1)], "huge.txt", {
			type: "text/plain",
		});

		act(() => {
			result.current.handleAttach([file]);
		});

		expect(uploadSpy).not.toHaveBeenCalled();
		expect(result.current.attachments).toHaveLength(1);
		expect(result.current.uploadStates.get(file)).toMatchObject({
			status: "error",
			error: expect.stringContaining("Maximum is 10 MiB"),
		});
		expect(localStorage.getItem(storageKey)).toBeNull();
		unmount();
	});

	it("resetAttachments clears storage, registry entries, and previews", async () => {
		const upload = createDeferred<{ id: string }>();
		vi.spyOn(API.experimental, "uploadChatFile").mockReturnValue(
			upload.promise,
		);
		const { result, unmount } = renderHook(() =>
			useChatDraftAttachments(orgID, chatID),
		);
		const file = new File(["hello"], "reset.png", {
			type: "image/png",
			lastModified: 14,
		});

		act(() => {
			result.current.handleAttach([file]);
		});
		await vi.waitFor(() => {
			expect(parseStoredDrafts()).toHaveLength(1);
		});

		act(() => {
			result.current.resetAttachments();
		});
		expect(result.current.attachments).toHaveLength(0);
		expect(localStorage.getItem(storageKey)).toBeNull();
		expect(URL.revokeObjectURL).toHaveBeenCalledWith("blob:attachment-preview");

		await act(async () => {
			upload.resolve({ id: "file-reset" });
		});
		expect(result.current.attachments).toHaveLength(0);
		expect(localStorage.getItem(storageKey)).toBeNull();
		unmount();
	});

	it("stale reset clears its original chat without clearing the current chat", async () => {
		const firstUpload = createDeferred<{ id: string }>();
		const secondUpload = createDeferred<{ id: string }>();
		vi.spyOn(API.experimental, "uploadChatFile")
			.mockReturnValueOnce(firstUpload.promise)
			.mockReturnValueOnce(secondUpload.promise);
		const secondChatID = "chat-b";
		const secondStorageKey = chatDraftAttachmentStorageKey(orgID, secondChatID);
		const { result, rerender, unmount } = renderHook(
			({ chatId }) => useChatDraftAttachments(orgID, chatId),
			{ initialProps: { chatId: chatID } },
		);
		const firstFile = new File(["first"], "first.txt", {
			type: "text/plain",
			lastModified: 15,
		});

		act(() => {
			result.current.handleAttach([firstFile]);
		});
		await vi.waitFor(() => {
			expect(parseStoredDrafts()).toHaveLength(1);
		});
		const resetFirstChat = result.current.resetAttachments;
		rerender({ chatId: secondChatID });
		const secondFile = new File(["second"], "second.txt", {
			type: "text/plain",
			lastModified: 16,
		});

		act(() => {
			result.current.handleAttach([secondFile]);
		});
		await vi.waitFor(() => {
			expect(localStorage.getItem(secondStorageKey)).not.toBeNull();
		});

		act(() => {
			resetFirstChat();
		});
		expect(localStorage.getItem(storageKey)).toBeNull();
		expect(localStorage.getItem(secondStorageKey)).not.toBeNull();
		expect(result.current.attachments).toHaveLength(1);
		expect(result.current.attachments[0].name).toBe("second.txt");

		await act(async () => {
			firstUpload.resolve({ id: "file-first" });
			secondUpload.resolve({ id: "file-second" });
		});
		await vi.waitFor(() => {
			expect(result.current.uploadStates.get(secondFile)).toMatchObject({
				status: "uploaded",
				fileId: "file-second",
			});
		});
		expect(result.current.attachments).toHaveLength(1);
		expect(localStorage.getItem(storageKey)).toBeNull();
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
				{
					status: "pending",
					clientId: "mismatched-payload",
					fileName: "bad.txt",
					fileType: "text/plain",
					lastModified: 5,
					size: 10,
					organizationId: orgID,
					chatId: chatID,
					payload: "data:text/html;base64,PGgxPkhlbGxvPC9oMT4=",
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

	describe("compose-path resize", () => {
		const makeOversizeImage = () =>
			new File([new Uint8Array(5 * 1024 * 1024 + 64 * 1024)], "photo.png", {
				type: "image/png",
				lastModified: 100,
			});

		it("commits the original synchronously with status: processing while resize is in flight", async () => {
			const resize = await import("../utils/resizeImage");
			vi.spyOn(resize, "resizeImageToMaxBytes").mockImplementation(
				() => new Promise<File | null>(() => undefined),
			);
			const uploadSpy = vi.spyOn(API.experimental, "uploadChatFile");

			const { result, unmount } = renderHook(() =>
				useChatDraftAttachments(orgID, chatID, { provider: "anthropic" }),
			);
			const original = makeOversizeImage();

			act(() => {
				result.current.handleAttach([original]);
			});

			expect(result.current.attachments).toHaveLength(1);
			expect(result.current.attachments[0]).toBe(original);
			expect(result.current.uploadStates.get(original)).toMatchObject({
				status: "processing",
			});
			// Registry entry is created post-resize.
			expect(uploadSpy).not.toHaveBeenCalled();
			expect(localStorage.getItem(storageKey)).toBeNull();
			unmount();
		});

		it("swaps the original for a smaller resized File and starts the upload", async () => {
			const upload = createDeferred<{ id: string }>();
			const uploadSpy = vi
				.spyOn(API.experimental, "uploadChatFile")
				.mockReturnValue(upload.promise);
			const resize = await import("../utils/resizeImage");
			const replacement = new File(
				[new Uint8Array(2 * 1024 * 1024)],
				"photo.webp",
				{ type: "image/webp", lastModified: 200 },
			);
			let releaseResize: (value: File | null) => void = () => undefined;
			vi.spyOn(resize, "resizeImageToMaxBytes").mockImplementation(
				() =>
					new Promise<File | null>((resolveFn) => {
						releaseResize = resolveFn;
					}),
			);

			const { result, unmount } = renderHook(() =>
				useChatDraftAttachments(orgID, chatID, { provider: "anthropic" }),
			);
			const original = makeOversizeImage();

			act(() => {
				result.current.handleAttach([original]);
			});
			expect(result.current.uploadStates.get(original)?.status).toBe(
				"processing",
			);

			await act(async () => {
				releaseResize(replacement);
				await Promise.resolve();
			});

			await vi.waitFor(() => {
				expect(result.current.attachments).toHaveLength(1);
				expect(result.current.attachments[0]).toBe(replacement);
			});
			expect(result.current.uploadStates.get(original)).toBeUndefined();
			expect(uploadSpy).toHaveBeenCalledTimes(1);
			expect(uploadSpy).toHaveBeenCalledWith(replacement, orgID);

			await act(async () => {
				upload.resolve({ id: "file-resized" });
			});
			await vi.waitFor(() => {
				expect(result.current.uploadStates.get(replacement)).toMatchObject({
					status: "uploaded",
					fileId: "file-resized",
				});
			});
			unmount();
		});

		it("falls back to the original and surfaces a provider-budget error when resize returns null on Anthropic", async () => {
			const uploadSpy = vi.spyOn(API.experimental, "uploadChatFile");
			const resize = await import("../utils/resizeImage");
			vi.spyOn(resize, "resizeImageToMaxBytes").mockResolvedValue(null);

			const { result, unmount } = renderHook(() =>
				useChatDraftAttachments(orgID, chatID, { provider: "anthropic" }),
			);
			const original = makeOversizeImage();

			act(() => {
				result.current.handleAttach([original]);
			});
			await vi.waitFor(() => {
				const state = result.current.uploadStates.get(original);
				expect(state?.status).toBe("error");
				expect(state?.error).toMatch(/Anthropic/);
				expect(state?.error).toMatch(/MiB/);
			});

			expect(uploadSpy).not.toHaveBeenCalled();
			expect(localStorage.getItem(storageKey)).toBeNull();
			unmount();
		});

		it("does not resurrect attachments removed while resize is in flight", async () => {
			const uploadSpy = vi.spyOn(API.experimental, "uploadChatFile");
			const resize = await import("../utils/resizeImage");
			let releaseResize: (value: File | null) => void = () => undefined;
			vi.spyOn(resize, "resizeImageToMaxBytes").mockImplementation(
				() =>
					new Promise<File | null>((resolveFn) => {
						releaseResize = resolveFn;
					}),
			);

			const { result, unmount } = renderHook(() =>
				useChatDraftAttachments(orgID, chatID, { provider: "anthropic" }),
			);
			const original = makeOversizeImage();

			act(() => {
				result.current.handleAttach([original]);
			});
			expect(result.current.attachments).toHaveLength(1);

			act(() => {
				result.current.handleRemoveAttachment(original);
			});
			expect(result.current.attachments).toHaveLength(0);

			const replacement = new File(
				[new Uint8Array(1 * 1024 * 1024)],
				"photo.webp",
				{ type: "image/webp" },
			);
			await act(async () => {
				releaseResize(replacement);
				await Promise.resolve();
			});

			expect(result.current.attachments).toHaveLength(0);
			expect(uploadSpy).not.toHaveBeenCalled();
			expect(localStorage.getItem(storageKey)).toBeNull();
			unmount();
		});

		it("does not resurrect attachments after resetAttachments fires", async () => {
			const uploadSpy = vi.spyOn(API.experimental, "uploadChatFile");
			const resize = await import("../utils/resizeImage");
			let releaseResize: (value: File | null) => void = () => undefined;
			vi.spyOn(resize, "resizeImageToMaxBytes").mockImplementation(
				() =>
					new Promise<File | null>((resolveFn) => {
						releaseResize = resolveFn;
					}),
			);

			const { result, unmount } = renderHook(() =>
				useChatDraftAttachments(orgID, chatID, { provider: "anthropic" }),
			);
			const original = makeOversizeImage();

			act(() => {
				result.current.handleAttach([original]);
			});
			expect(result.current.attachments).toHaveLength(1);

			act(() => {
				result.current.resetAttachments();
			});
			expect(result.current.attachments).toHaveLength(0);

			const replacement = new File(
				[new Uint8Array(1 * 1024 * 1024)],
				"photo.webp",
				{ type: "image/webp" },
			);
			await act(async () => {
				releaseResize(replacement);
				await Promise.resolve();
			});

			expect(result.current.attachments).toHaveLength(0);
			expect(uploadSpy).not.toHaveBeenCalled();
			expect(localStorage.getItem(storageKey)).toBeNull();
			unmount();
		});

		it("freezes the provider snapshot at attach time so a mid-resize provider switch can't mislabel the error", async () => {
			const resize = await import("../utils/resizeImage");
			let releaseResize: (value: File | null) => void = () => undefined;
			vi.spyOn(resize, "resizeImageToMaxBytes").mockImplementation(
				() =>
					new Promise<File | null>((resolveFn) => {
						releaseResize = resolveFn;
					}),
			);

			const { result, rerender, unmount } = renderHook(
				({ provider }) => useChatDraftAttachments(orgID, chatID, { provider }),
				{ initialProps: { provider: "anthropic" } },
			);

			// Over Anthropic's 5 MiB but under OpenAI's 10 MiB.
			const gif = new File([new Uint8Array(8)], "animated.gif", {
				type: "image/gif",
				lastModified: 400,
			});
			Object.defineProperty(gif, "size", { value: 6 * 1024 * 1024 });

			act(() => {
				result.current.handleAttach([gif]);
			});

			rerender({ provider: "openai" });

			await act(async () => {
				releaseResize(null);
				await Promise.resolve();
			});

			// Error must name the provider whose budget rejected
			// the file at attach time, not the live provider.
			await vi.waitFor(() => {
				const state = result.current.uploadStates.get(gif);
				expect(state?.status).toBe("error");
				expect(state?.error).toMatch(/Anthropic/);
				expect(state?.error).not.toMatch(/OpenAI/);
				expect(state?.error).toMatch(/under 5\.0 MiB/);
			});
			unmount();
		});

		it("uses the default 10 MiB budget for non-Anthropic providers (no resize for sub-10MiB images)", async () => {
			const upload = createDeferred<{ id: string }>();
			const uploadSpy = vi
				.spyOn(API.experimental, "uploadChatFile")
				.mockReturnValue(upload.promise);
			const resize = await import("../utils/resizeImage");
			const resizeSpy = vi
				.spyOn(resize, "resizeImageToMaxBytes")
				.mockResolvedValue(null);

			const { result, unmount } = renderHook(() =>
				useChatDraftAttachments(orgID, chatID, { provider: "openai" }),
			);
			// 7 MiB: over Anthropic's 5 MiB but under the default
			// 10 MiB. OpenAI uploads directly without resize.
			const file = new File([new Uint8Array(7 * 1024 * 1024)], "medium.png", {
				type: "image/png",
				lastModified: 300,
			});

			act(() => {
				result.current.handleAttach([file]);
			});

			expect(resizeSpy).not.toHaveBeenCalled();
			await vi.waitFor(() => {
				expect(uploadSpy).toHaveBeenCalledTimes(1);
				expect(uploadSpy).toHaveBeenCalledWith(file, orgID);
			});

			await act(async () => {
				upload.resolve({ id: "file-direct" });
			});
			await vi.waitFor(() => {
				expect(result.current.uploadStates.get(file)).toMatchObject({
					status: "uploaded",
					fileId: "file-direct",
				});
			});
			unmount();
		});
	});
});
