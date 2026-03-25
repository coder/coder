import { act, renderHook } from "@testing-library/react";
import { afterEach, beforeEach, describe, expect, it, vi } from "vitest";
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
			result.current.handleContentChange("work in progress");
		});

		expect(localStorage.getItem(emptyInputStorageKey)).toBe("work in progress");
		expect(result.current.getCurrentContent()).toBe("work in progress");
		unmount();
	});

	it("removes the draft key when handleContentChange receives empty string", () => {
		localStorage.setItem(emptyInputStorageKey, "old draft");
		const { result, unmount } = renderDraft();

		act(() => {
			result.current.handleContentChange("");
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
			result.current.handleContentChange("fix the bug");
		});

		expect(localStorage.getItem(emptyInputStorageKey)).toBeNull();
		expect(result.current.getCurrentContent()).toBe("fix the bug");
		unmount();
	});

	it("ignores all handleContentChange calls after submitDraft, even with new content", () => {
		const { result, unmount } = renderDraft();

		act(() => {
			result.current.handleContentChange("original");
		});
		expect(localStorage.getItem(emptyInputStorageKey)).toBe("original");

		act(() => {
			result.current.submitDraft();
		});

		act(() => {
			result.current.handleContentChange("totally new content");
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
			result.current.handleContentChange("attempt one");
		});
		expect(localStorage.getItem(emptyInputStorageKey)).toBe("attempt one");

		act(() => {
			result.current.submitDraft();
		});
		expect(localStorage.getItem(emptyInputStorageKey)).toBeNull();

		// Simulate error recovery — re-enable persistence.
		act(() => {
			result.current.resetDraft();
		});

		act(() => {
			result.current.handleContentChange("attempt two");
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
		}> = {},
	) => ({
		fileId: "file-1",
		fileName: "photo.png",
		fileType: "image/png",
		lastModified: 1000,
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
});
