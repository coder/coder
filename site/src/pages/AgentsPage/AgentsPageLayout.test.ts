import { act, renderHook } from "@testing-library/react";
import { afterEach, beforeEach, describe, expect, it, vi } from "vitest";
import type * as TypesGen from "#/api/typesGenerated";
import { shouldInvalidateFilteredChatList } from "./AgentsPageLayout";
import {
	emptyInputStorageKey,
	useEmptyStateDraft,
} from "./components/AgentCreateForm";
import {
	persistedAttachmentsStorageKey,
	useFileAttachments,
} from "./hooks/useFileAttachments";

describe("useEmptyStateDraft", () => {
	beforeEach(() => {
		localStorage.clear();
	});

	const renderDraft = () => renderHook(() => useEmptyStateDraft());

	it("reads the initial value from localStorage", () => {
		localStorage.setItem(emptyInputStorageKey, "saved draft");

		const { result, unmount } = renderDraft();

		expect(result.current.initialInputValue).toBe("saved draft");
		expect(result.current.getCurrentContent()).toBe("saved draft");
		unmount();
	});

	it("returns empty string when localStorage has no draft", () => {
		const { result, unmount } = renderDraft();

		expect(result.current.initialInputValue).toBe("");
		expect(result.current.getCurrentContent()).toBe("");
		unmount();
	});

	it("writes content to localStorage via handleContentChange", () => {
		const { result, unmount } = renderDraft();

		act(() => {
			result.current.handleContentChange(
				"work in progress",
				"work in progress",
				false,
			);
		});

		expect(localStorage.getItem(emptyInputStorageKey)).toBe("work in progress");
		expect(result.current.getCurrentContent()).toBe("work in progress");
		unmount();
	});

	it("removes the draft key when handleContentChange receives empty string", () => {
		localStorage.setItem(emptyInputStorageKey, "old draft");
		const { result, unmount } = renderDraft();

		act(() => {
			// Even though the serialized state is non-empty (Lexical always
			// produces a JSON object), the draft is removed when the plain
			// text content is empty.
			result.current.handleContentChange("", '{"root":{"children":[]}}', false);
		});

		expect(localStorage.getItem(emptyInputStorageKey)).toBeNull();
		unmount();
	});

	it("clears the draft from localStorage when submitDraft is called", () => {
		localStorage.setItem(emptyInputStorageKey, "draft to clear");
		const { result, unmount } = renderDraft();

		expect(localStorage.getItem(emptyInputStorageKey)).toBe("draft to clear");

		act(() => {
			result.current.submitDraft();
		});

		expect(localStorage.getItem(emptyInputStorageKey)).toBeNull();
		unmount();
	});

	it("does not re-persist the draft after submitDraft is called", () => {
		localStorage.setItem(emptyInputStorageKey, "fix the bug");
		const { result, unmount } = renderDraft();

		// Simulate handleSend: submitDraft clears the draft.
		act(() => {
			result.current.submitDraft();
		});
		expect(localStorage.getItem(emptyInputStorageKey)).toBeNull();

		// Simulate the Lexical ContentChangePlugin firing during
		// the re-render with the old content. Without the sentRef
		// guard this would re-persist the draft.
		act(() => {
			result.current.handleContentChange("fix the bug", "fix the bug", false);
		});

		expect(localStorage.getItem(emptyInputStorageKey)).toBeNull();
		expect(result.current.getCurrentContent()).toBe("fix the bug");
		unmount();
	});

	it("ignores all handleContentChange calls after submitDraft, even with new content", () => {
		const { result, unmount } = renderDraft();

		act(() => {
			result.current.handleContentChange("original", "original", false);
		});
		expect(localStorage.getItem(emptyInputStorageKey)).toBe("original");

		act(() => {
			result.current.submitDraft();
		});

		act(() => {
			result.current.handleContentChange(
				"totally new content",
				"totally new content",
				false,
			);
		});
		expect(localStorage.getItem(emptyInputStorageKey)).toBeNull();
		unmount();
	});

	it("returns empty draft when remounting after submitDraft", () => {
		localStorage.setItem(emptyInputStorageKey, "draft before send");
		const { result, unmount } = renderDraft();

		act(() => {
			result.current.submitDraft();
		});
		unmount();

		// Simulate returning to the page.
		const { result: fresh, unmount: unmountFresh } = renderDraft();
		expect(fresh.current.initialInputValue).toBe("");
		expect(localStorage.getItem(emptyInputStorageKey)).toBeNull();
		unmountFresh();
	});

	it("re-enables draft persistence after resetDraft", () => {
		const { result, unmount } = renderDraft();

		act(() => {
			result.current.handleContentChange("attempt one", "attempt one", false);
		});
		expect(localStorage.getItem(emptyInputStorageKey)).toBe("attempt one");

		act(() => {
			result.current.submitDraft();
		});
		expect(localStorage.getItem(emptyInputStorageKey)).toBeNull();

		// Simulate error recovery -- re-enable persistence.
		act(() => {
			result.current.resetDraft();
		});

		act(() => {
			result.current.handleContentChange("attempt two", "attempt two", false);
		});
		expect(localStorage.getItem(emptyInputStorageKey)).toBe("attempt two");
		unmount();
	});

	it("handles submitDraft being called twice without error", () => {
		localStorage.setItem(emptyInputStorageKey, "draft");
		const { result, unmount } = renderDraft();

		act(() => {
			result.current.submitDraft();
		});
		expect(localStorage.getItem(emptyInputStorageKey)).toBeNull();

		act(() => {
			result.current.submitDraft();
		});
		expect(localStorage.getItem(emptyInputStorageKey)).toBeNull();
		unmount();
	});

	it("persists serialized editor state when provided", () => {
		const { result, unmount } = renderDraft();
		const editorState = JSON.stringify({
			root: {
				children: [
					{
						children: [
							{ text: "review this" },
							{
								type: "file-reference",
								version: 1,
								fileName: "main.go",
								startLine: 1,
								endLine: 10,
								content: "code",
							},
						],
						type: "paragraph",
					},
				],
				type: "root",
			},
		});

		act(() => {
			result.current.handleContentChange("review this", editorState, true);
		});

		expect(localStorage.getItem(emptyInputStorageKey)).toBe(editorState);
		expect(result.current.getCurrentContent()).toBe("review this");
		unmount();
	});

	it("restores initialEditorState from a Lexical JSON draft", () => {
		const editorState = JSON.stringify({
			root: {
				children: [
					{
						children: [{ text: "hello" }],
						type: "paragraph",
					},
				],
				type: "root",
			},
		});
		localStorage.setItem(emptyInputStorageKey, editorState);

		const { result, unmount } = renderDraft();

		expect(result.current.initialEditorState).toBe(editorState);
		expect(result.current.initialInputValue).toBe("hello");
		unmount();
	});

	it("falls back to plain text for legacy drafts", () => {
		localStorage.setItem(emptyInputStorageKey, "legacy plain text");

		const { result, unmount } = renderDraft();

		expect(result.current.initialEditorState).toBeUndefined();
		expect(result.current.initialInputValue).toBe("legacy plain text");
		unmount();
	});

	it("persists file-reference-only drafts (no text content)", () => {
		const { result, unmount } = renderDraft();
		const editorState = JSON.stringify({
			root: {
				children: [
					{
						children: [
							{
								type: "file-reference",
								version: 1,
								fileName: "main.go",
								startLine: 1,
								endLine: 10,
								content: "code",
							},
						],
						type: "paragraph",
					},
				],
				type: "root",
			},
		});

		act(() => {
			result.current.handleContentChange("", editorState, true);
		});

		expect(localStorage.getItem(emptyInputStorageKey)).toBe(editorState);
		unmount();
	});

	it("removes draft for whitespace-only content without file references", () => {
		localStorage.setItem(emptyInputStorageKey, "old draft");
		const { result, unmount } = renderDraft();

		act(() => {
			result.current.handleContentChange("   ", '{"root":{}}', false);
		});

		expect(localStorage.getItem(emptyInputStorageKey)).toBeNull();
		unmount();
	});
});

describe("useFileAttachments persistence", () => {
	beforeEach(() => {
		localStorage.clear();
	});

	afterEach(() => {
		vi.restoreAllMocks();
	});

	const renderFileAttachments = () =>
		renderHook(() => useFileAttachments("org-1", { persist: true }));

	const makePersistedEntry = (
		overrides: Partial<{
			fileId: string;
			fileName: string;
			fileType: string;
			lastModified: number;
			organizationId: string;
		}> = {},
	) => ({
		fileId: "file-1",
		fileName: "photo.png",
		fileType: "image/png",
		lastModified: 1000,
		organizationId: "org-1",
		...overrides,
	});

	it("restores uploaded attachments from localStorage on mount", () => {
		const entry = makePersistedEntry();
		localStorage.setItem(
			persistedAttachmentsStorageKey,
			JSON.stringify([entry]),
		);

		const { result, unmount } = renderFileAttachments();

		expect(result.current.attachments).toHaveLength(1);
		expect(result.current.attachments[0].name).toBe("photo.png");
		expect(result.current.attachments[0].type).toBe("image/png");

		const file = result.current.attachments[0];
		const state = result.current.uploadStates.get(file);
		expect(state).toEqual({ status: "uploaded", fileId: "file-1" });

		const previewUrl = result.current.previewUrls.get(file);
		expect(previewUrl).toBe("/api/experimental/chats/files/file-1");
		unmount();
	});

	it("does not create preview URLs for non-image attachments", () => {
		const entry = makePersistedEntry({
			fileType: "text/plain",
			fileName: "notes.txt",
		});
		localStorage.setItem(
			persistedAttachmentsStorageKey,
			JSON.stringify([entry]),
		);

		const { result, unmount } = renderFileAttachments();

		expect(result.current.attachments).toHaveLength(1);
		const file = result.current.attachments[0];
		expect(result.current.previewUrls.has(file)).toBe(false);
		expect(result.current.uploadStates.get(file)).toEqual({
			status: "uploaded",
			fileId: "file-1",
		});
		unmount();
	});

	it("returns empty state when nothing is persisted", () => {
		const { result, unmount } = renderFileAttachments();

		expect(result.current.attachments).toHaveLength(0);
		expect(result.current.uploadStates.size).toBe(0);
		expect(result.current.previewUrls.size).toBe(0);
		unmount();
	});

	it("does not restore when persist option is false", () => {
		const entry = makePersistedEntry();
		localStorage.setItem(
			persistedAttachmentsStorageKey,
			JSON.stringify([entry]),
		);

		const { result, unmount } = renderHook(() =>
			useFileAttachments("org-1", { persist: false }),
		);

		expect(result.current.attachments).toHaveLength(0);
		unmount();
	});

	it("does not restore when no options argument is passed", () => {
		const entry = makePersistedEntry();
		localStorage.setItem(
			persistedAttachmentsStorageKey,
			JSON.stringify([entry]),
		);

		const { result, unmount } = renderHook(() => useFileAttachments("org-1"));

		expect(result.current.attachments).toHaveLength(0);
		unmount();
	});

	it("clears persisted attachments on resetAttachments", () => {
		const entry = makePersistedEntry();
		localStorage.setItem(
			persistedAttachmentsStorageKey,
			JSON.stringify([entry]),
		);

		const { result, unmount } = renderFileAttachments();

		act(() => {
			result.current.resetAttachments();
		});

		expect(localStorage.getItem(persistedAttachmentsStorageKey)).toBeNull();
		expect(result.current.attachments).toHaveLength(0);
		unmount();
	});

	it("removes the correct entry when an attachment is removed", () => {
		const entries = [
			makePersistedEntry({ fileId: "file-1", fileName: "a.png" }),
			makePersistedEntry({ fileId: "file-2", fileName: "b.png" }),
		];
		localStorage.setItem(
			persistedAttachmentsStorageKey,
			JSON.stringify(entries),
		);

		const { result, unmount } = renderFileAttachments();
		expect(result.current.attachments).toHaveLength(2);

		act(() => {
			result.current.handleRemoveAttachment(0);
		});

		expect(result.current.attachments).toHaveLength(1);
		expect(result.current.attachments[0].name).toBe("b.png");

		const stored = JSON.parse(
			localStorage.getItem(persistedAttachmentsStorageKey)!,
		);
		expect(stored).toHaveLength(1);
		expect(stored[0].fileId).toBe("file-2");
		unmount();
	});

	it("handles corrupt localStorage gracefully", () => {
		localStorage.setItem(persistedAttachmentsStorageKey, "not-valid-json");

		const { result, unmount } = renderFileAttachments();

		expect(result.current.attachments).toHaveLength(0);
		unmount();
	});

	it("persists attachment metadata after successful upload", async () => {
		const { API } = await import("#/api/api");
		vi.spyOn(API.experimental, "uploadChatFile").mockResolvedValue({
			id: "new-file-id",
		});
		vi.spyOn(globalThis, "fetch").mockResolvedValue(new Response());

		const { result, unmount } = renderFileAttachments();

		const file = new File(["hello"], "test.png", { type: "image/png" });

		act(() => {
			result.current.handleAttach([file]);
		});

		// Wait for the async upload to complete and state to update.
		await vi.waitFor(() => {
			const state = result.current.uploadStates.get(file);
			expect(state?.status).toBe("uploaded");
		});

		const stored = JSON.parse(
			localStorage.getItem(persistedAttachmentsStorageKey)!,
		);
		expect(stored).toHaveLength(1);
		expect(stored[0].fileId).toBe("new-file-id");
		expect(stored[0].fileName).toBe("test.png");
		unmount();
	});

	it("does not persist attachment metadata when upload fails", async () => {
		const { API } = await import("#/api/api");
		vi.spyOn(API.experimental, "uploadChatFile").mockRejectedValue(
			new Error("server error"),
		);

		const { result, unmount } = renderFileAttachments();

		const file = new File(["hello"], "test.png", { type: "image/png" });

		act(() => {
			result.current.handleAttach([file]);
		});

		await vi.waitFor(() => {
			const state = result.current.uploadStates.get(file);
			expect(state?.status).toBe("error");
		});

		expect(localStorage.getItem(persistedAttachmentsStorageKey)).toBeNull();
		unmount();
	});

	it("restores only attachments matching current organization", () => {
		const entries = [
			makePersistedEntry({
				fileId: "f1",
				fileName: "a.png",
				organizationId: "org-1",
			}),
			makePersistedEntry({
				fileId: "f2",
				fileName: "b.png",
				organizationId: "org-2",
			}),
		];
		localStorage.setItem(
			persistedAttachmentsStorageKey,
			JSON.stringify(entries),
		);

		const { result, unmount } = renderFileAttachments();

		expect(result.current.attachments).toHaveLength(1);
		expect(result.current.attachments[0].name).toBe("a.png");

		// localStorage should be pruned to only the matching org.
		const stored = JSON.parse(
			localStorage.getItem(persistedAttachmentsStorageKey)!,
		);
		expect(stored).toHaveLength(1);
		expect(stored[0].fileId).toBe("f1");
		unmount();
	});

	it("prunes legacy entries without organizationId", () => {
		const legacy = {
			fileId: "old-file",
			fileName: "legacy.png",
			fileType: "image/png",
			lastModified: 1000,
			// No organizationId field -- simulates pre-org-scoping data.
		};
		localStorage.setItem(
			persistedAttachmentsStorageKey,
			JSON.stringify([legacy]),
		);

		const { result, unmount } = renderFileAttachments();

		expect(result.current.attachments).toHaveLength(0);
		expect(localStorage.getItem(persistedAttachmentsStorageKey)).toBeNull();
		unmount();
	});

	it("skips restoration when organizationId is undefined", () => {
		const entry = makePersistedEntry();
		localStorage.setItem(
			persistedAttachmentsStorageKey,
			JSON.stringify([entry]),
		);

		const { result, unmount } = renderHook(() =>
			useFileAttachments(undefined, { persist: true }),
		);

		// Should not restore -- org not yet known.
		expect(result.current.attachments).toHaveLength(0);
		// Should NOT prune -- org unknown, so leave storage alone.
		expect(localStorage.getItem(persistedAttachmentsStorageKey)).not.toBeNull();
		unmount();
	});

	it("persists organizationId with attachment metadata", async () => {
		const { API } = await import("#/api/api");
		vi.spyOn(API.experimental, "uploadChatFile").mockResolvedValue({
			id: "new-file-id",
		});
		vi.spyOn(globalThis, "fetch").mockResolvedValue(new Response());

		const { result, unmount } = renderFileAttachments();

		const file = new File(["hello"], "test.png", { type: "image/png" });

		act(() => {
			result.current.handleAttach([file]);
		});

		await vi.waitFor(() => {
			const state = result.current.uploadStates.get(file);
			expect(state?.status).toBe("uploaded");
		});

		const stored = JSON.parse(
			localStorage.getItem(persistedAttachmentsStorageKey)!,
		);
		expect(stored).toHaveLength(1);
		expect(stored[0].organizationId).toBe("org-1");
		unmount();
	});
});

// Synthetic resize swaps the File without invoking the browser's
// decoder, so these tests run in jsdom without resizeImage.ts fakes.
describe("useFileAttachments processResizes", () => {
	beforeEach(() => {
		localStorage.clear();
	});

	afterEach(() => {
		vi.restoreAllMocks();
	});

	// Reported size > budget without actually allocating bytes.
	const makeOversizeImage = (
		name = "photo.png",
		type = "image/png",
		bytes = 6 * 1024 * 1024,
	) => {
		const file = new File([new Uint8Array(8)], name, { type });
		Object.defineProperty(file, "size", { value: bytes });
		return file;
	};

	it("marks oversize images as processing synchronously on attach", async () => {
		const resize = await import("./utils/resizeImage");
		// Never-resolving resize so "processing" stays observable.
		vi.spyOn(resize, "resizeImageToMaxBytes").mockImplementation(
			() => new Promise(() => undefined),
		);

		const { result, unmount } = renderHook(() =>
			useFileAttachments("org-1", { provider: "anthropic" }),
		);

		const file = makeOversizeImage();

		act(() => {
			result.current.handleAttach([file]);
		});

		expect(result.current.attachments).toHaveLength(1);
		expect(result.current.attachments[0]).toBe(file);
		const state = result.current.uploadStates.get(file);
		expect(state?.status).toBe("processing");
		unmount();
	});

	it("swaps the original File for the resized replacement when resize succeeds", async () => {
		const resize = await import("./utils/resizeImage");
		const { API } = await import("#/api/api");
		const replacement = new File([new Uint8Array(1024)], "photo.webp", {
			type: "image/webp",
		});
		vi.spyOn(resize, "resizeImageToMaxBytes").mockResolvedValue(replacement);
		vi.spyOn(API.experimental, "uploadChatFile").mockResolvedValue({
			id: "file-id",
		});
		vi.spyOn(globalThis, "fetch").mockResolvedValue(new Response());

		const { result, unmount } = renderHook(() =>
			useFileAttachments("org-1", { provider: "anthropic" }),
		);

		const original = makeOversizeImage();

		act(() => {
			result.current.handleAttach([original]);
		});

		await vi.waitFor(() => {
			expect(result.current.attachments[0]).toBe(replacement);
		});

		await vi.waitFor(() => {
			expect(result.current.uploadStates.get(replacement)?.status).toBe(
				"uploaded",
			);
		});
		expect(result.current.uploadStates.get(original)).toBeUndefined();
		unmount();
	});

	it("falls back to the original File when resize returns null", async () => {
		const resize = await import("./utils/resizeImage");
		const { API } = await import("#/api/api");
		vi.spyOn(resize, "resizeImageToMaxBytes").mockResolvedValue(null);
		vi.spyOn(API.experimental, "uploadChatFile").mockResolvedValue({
			id: "file-id",
		});
		vi.spyOn(globalThis, "fetch").mockResolvedValue(new Response());

		// 6 MiB on Anthropic + null resize forces the
		// provider-budget-error path.
		const { result, unmount } = renderHook(() =>
			useFileAttachments("org-1", { provider: "anthropic" }),
		);

		const original = makeOversizeImage(
			"photo.png",
			"image/png",
			6 * 1024 * 1024,
		);

		act(() => {
			result.current.handleAttach([original]);
		});

		await vi.waitFor(() => {
			const state = result.current.uploadStates.get(original);
			expect(state?.status).toBe("error");
			expect(state?.error).toMatch(/Anthropic/);
			expect(state?.error).toMatch(/MiB/);
		});
		unmount();
	});

	it("freezes the provider snapshot at attach time so a mid-resize provider switch can't mislabel the error", async () => {
		const resize = await import("./utils/resizeImage");
		let releaseResize: (value: File | null) => void = () => undefined;
		vi.spyOn(resize, "resizeImageToMaxBytes").mockImplementation(
			() =>
				new Promise<File | null>((resolve) => {
					releaseResize = resolve;
				}),
		);

		const { result, rerender, unmount } = renderHook(
			({ provider }) => useFileAttachments("org-1", { provider }),
			{ initialProps: { provider: "anthropic" } },
		);

		// Attach on Anthropic (5 MiB budget).
		const original = makeOversizeImage(
			"photo.gif",
			"image/gif",
			6 * 1024 * 1024,
		);
		act(() => {
			result.current.handleAttach([original]);
		});

		// User switches to OpenAI (10 MiB budget) before the
		// resize finishes. providerRef.current is now "openai".
		rerender({ provider: "openai" });

		// Resize gives up (animated GIF; falls back to original).
		await act(async () => {
			releaseResize(null);
			await Promise.resolve();
		});

		// Error must name Anthropic (the provider whose budget
		// rejected the file), not OpenAI (the live ref). The
		// budget value in the error must match Anthropic's
		// ~5 MiB, not OpenAI's ~10 MiB.
		await vi.waitFor(() => {
			const state = result.current.uploadStates.get(original);
			expect(state?.status).toBe("error");
			expect(state?.error).toMatch(/Anthropic/);
			expect(state?.error).not.toMatch(/OpenAI/);
			expect(state?.error).toMatch(/under 5\.0 MiB/);
		});
		unmount();
	});

	it("does not resurrect attachments removed while resize is in flight", async () => {
		const resize = await import("./utils/resizeImage");
		let releaseResize: (value: File | null) => void = () => undefined;
		vi.spyOn(resize, "resizeImageToMaxBytes").mockImplementation(
			() =>
				new Promise<File | null>((resolve) => {
					releaseResize = resolve;
				}),
		);

		const { result, unmount } = renderHook(() =>
			useFileAttachments("org-1", { provider: "anthropic" }),
		);

		const original = makeOversizeImage();

		act(() => {
			result.current.handleAttach([original]);
		});
		expect(result.current.attachments).toHaveLength(1);

		// Remove the attachment while resize is still pending.
		act(() => {
			result.current.handleRemoveAttachment(original);
		});
		expect(result.current.attachments).toHaveLength(0);

		// Resolve the pending resize with a swap; the hook must
		// NOT resurrect the dismissed attachment.
		const replacement = new File([new Uint8Array(512)], "photo.webp", {
			type: "image/webp",
		});
		await act(async () => {
			releaseResize(replacement);
			await Promise.resolve();
		});

		expect(result.current.attachments).toHaveLength(0);
		expect(result.current.uploadStates.get(replacement)).toBeUndefined();
		unmount();
	});

	it("does not resurrect attachments after resetAttachments fires", async () => {
		const resize = await import("./utils/resizeImage");
		let releaseResize: (value: File | null) => void = () => undefined;
		vi.spyOn(resize, "resizeImageToMaxBytes").mockImplementation(
			() =>
				new Promise<File | null>((resolve) => {
					releaseResize = resolve;
				}),
		);

		const { result, unmount } = renderHook(() =>
			useFileAttachments("org-1", { provider: "anthropic" }),
		);

		const original = makeOversizeImage();

		act(() => {
			result.current.handleAttach([original]);
		});
		expect(result.current.attachments).toHaveLength(1);

		// Simulate a chat-scope reset (e.g. ChatPageContent's
		// editScopeRef effect when navigating between chats)
		// while the resize is still pending.
		act(() => {
			result.current.resetAttachments();
		});
		expect(result.current.attachments).toHaveLength(0);

		// Resolve the pending resize with a swap. The hook must
		// not re-add the replacement to attachments or kick off
		// an upload against the now-cleared scope.
		const replacement = new File([new Uint8Array(512)], "photo.webp", {
			type: "image/webp",
		});
		await act(async () => {
			releaseResize(replacement);
			await Promise.resolve();
		});

		expect(result.current.attachments).toHaveLength(0);
		expect(result.current.uploadStates.get(replacement)).toBeUndefined();
		expect(result.current.uploadStates.get(original)).toBeUndefined();
		unmount();
	});

	it("gates the send-reachable isUploading state on processing", () => {
		const resize = import("./utils/resizeImage");
		vi.spyOn(globalThis, "fetch").mockResolvedValue(new Response());
		// Never-resolving resize keeps the attachment in "processing".
		void resize.then((m) =>
			vi
				.spyOn(m, "resizeImageToMaxBytes")
				.mockImplementation(() => new Promise(() => undefined)),
		);

		const { result, unmount } = renderHook(() =>
			useFileAttachments("org-1", { provider: "anthropic" }),
		);

		const file = makeOversizeImage();

		act(() => {
			result.current.handleAttach([file]);
		});

		// Upload state must be "processing" (not "uploaded" or
		// undefined). The AgentChatInput send gate treats this the
		// same as "uploading" and blocks dispatch.
		expect(result.current.uploadStates.get(file)?.status).toBe("processing");
		unmount();
	});
});

const chatForFilterInvalidation = (
	overrides: Partial<TypesGen.Chat> = {},
): TypesGen.Chat =>
	({
		id: "chat-1",
		archived: false,
		parent_chat_id: null,
		...overrides,
	}) as TypesGen.Chat;

describe(shouldInvalidateFilteredChatList.name, () => {
	it.each<{
		name: string;
		updatedChat: TypesGen.Chat;
		eventKind: TypesGen.ChatWatchEventKind;
		expected: boolean;
	}>([
		{
			name: "invalidates root chats for membership events",
			updatedChat: chatForFilterInvalidation(),
			eventKind: "diff_status_change",
			expected: true,
		},
		{
			name: "ignores non-membership events",
			updatedChat: chatForFilterInvalidation(),
			eventKind: "title_change",
			expected: false,
		},
		{
			name: "excludes child chats",
			updatedChat: chatForFilterInvalidation({ parent_chat_id: "parent-1" }),
			eventKind: "diff_status_change",
			expected: false,
		},
	])("$name", ({ updatedChat, eventKind, expected }) => {
		expect(shouldInvalidateFilteredChatList(updatedChat, eventKind)).toBe(
			expected,
		);
	});
});
