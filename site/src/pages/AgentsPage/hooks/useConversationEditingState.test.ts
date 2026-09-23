import { act, renderHook } from "@testing-library/react";
import { createRef, useState } from "react";
import { beforeEach, describe, expect, it, vi } from "vitest";
import type { ChatMessagePart } from "#/api/typesGenerated";
import type { ChatMessageInputRef } from "../components/AgentChatInput";
import type { EditingTarget } from "../components/ChatConversation/types";
import type { PendingAttachment } from "../components/ChatPageContent";
import { draftInputStorageKeyPrefix } from "../utils/draftStorage";
import { useConversationEditingState } from "./useConversationEditingState";
import {
	type ComposerMode,
	deriveComposerTarget,
	type MarkerRequest,
} from "./useQueuedMessageEdit";

const idleMarker: MarkerRequest = { isPending: false, variables: undefined };

const textContent = (
	text: string,
	fileBlocks: readonly ChatMessagePart[] = [],
): readonly ChatMessagePart[] => [{ type: "text", text }, ...fileBlocks];

type MockChatInputHandle = {
	handle: ChatMessageInputRef;
	setValue: ReturnType<typeof vi.fn>;
	clear: ReturnType<typeof vi.fn>;
	focus: ReturnType<typeof vi.fn>;
	getValue: ReturnType<typeof vi.fn>;
	currentValue: { value: string };
};

const createMockChatInputHandle = (initialValue = ""): MockChatInputHandle => {
	const currentValue = { value: initialValue };
	const setValue = vi.fn((text: string) => {
		currentValue.value = text;
	});
	const clear = vi.fn(() => {
		currentValue.value = "";
	});
	const focus = vi.fn();
	const getValue = vi.fn(() => currentValue.value);

	return {
		handle: {
			setValue,
			insertText: vi.fn(),
			clear,
			focus,
			getValue,
			addFileReference: vi.fn(),
			getContentParts: vi.fn(() => []),
		},
		setValue,
		clear,
		focus,
		getValue,
		currentValue,
	};
};

const setMobileViewport = (isMobile: boolean) => {
	Object.defineProperty(window, "matchMedia", {
		writable: true,
		value: vi.fn((query: string): MediaQueryList => {
			return {
				matches: query === "(max-width: 639px)" ? isMobile : false,
				media: query,
				onchange: null,
				addEventListener: vi.fn(),
				removeEventListener: vi.fn(),
				dispatchEvent: vi.fn(() => true),
				addListener: vi.fn(),
				removeListener: vi.fn(),
			} as MediaQueryList;
		}),
	});
};

describe("useConversationEditingState", () => {
	const chatID = "chat-abc-123";
	const expectedKey = `${draftInputStorageKeyPrefix}${chatID}`;

	beforeEach(() => {
		localStorage.clear();
		setMobileViewport(false);
	});

	// The harness owns composerMode and derives the target the way the page
	// does: from the mode, the store marker and the marker request.
	type ServerProps = {
		serverMarkedID: number | null;
		marker: MarkerRequest;
	};
	const renderEditing = (...args: [] | [string | undefined]) => {
		const onSend = vi.fn().mockResolvedValue(undefined);
		const chatInputRef = createRef<ChatMessageInputRef>();
		const inputValueRef = { current: "" };
		const contents = new Map<string, readonly ChatMessagePart[]>();
		const contentKey = (target: EditingTarget) => `${target.kind}:${target.id}`;

		const resolvedChatID = args.length === 0 ? chatID : args[0];
		const initialProps: ServerProps = {
			serverMarkedID: null,
			marker: idleMarker,
		};

		const hook = renderHook(
			(props: ServerProps) => {
				const [composerMode, setComposerMode] = useState<ComposerMode>();
				const target = deriveComposerTarget(
					composerMode,
					props.serverMarkedID,
					props.marker,
					true,
				);
				const editing = useConversationEditingState({
					chatID: resolvedChatID,
					onSend,
					chatInputRef,
					inputValueRef,
					composerMode,
					setComposerMode,
					target,
					targetContent: target ? contents.get(contentKey(target)) : undefined,
				});
				return { ...editing, composerMode, setComposerMode };
			},
			{ initialProps },
		);

		// Callers wrap this in act. A queued target is marked on the server
		// first, as a settled begin request does on the page.
		const beginEdit = (
			target: EditingTarget,
			text: string,
			fileBlocks?: readonly ChatMessagePart[],
		) => {
			contents.set(contentKey(target), textContent(text, fileBlocks));
			if (target.kind === "queued") {
				hook.rerender({ serverMarkedID: target.id, marker: idleMarker });
			}
			hook.result.current.setComposerMode(target);
		};
		// The store marks a row, or none, with the latest marker request.
		const markOnServer = (
			id: number | null,
			text = "queued text",
			marker = idleMarker,
		) => {
			if (id !== null) {
				contents.set(contentKey({ kind: "queued", id }), textContent(text));
			}
			hook.rerender({ serverMarkedID: id, marker });
		};

		return { ...hook, onSend, inputValueRef, beginEdit, markOnServer };
	};

	it("persists and removes drafts via handleContentChange", () => {
		const { result, unmount } = renderEditing();

		act(() => {
			result.current.handleContentChange(
				"work in progress",
				"work in progress",
				false,
			);
		});
		expect(localStorage.getItem(expectedKey)).toBe("work in progress");
		// handleContentChange persists only; it must not advance the seed.
		expect(result.current.editorInitialValue).toBe("");
		expect(result.current.initialEditorState).toBeUndefined();

		act(() => {
			// Even though the serialized state is non-empty (Lexical always
			// produces a JSON object), the draft is removed when the plain
			// text content is empty.
			result.current.handleContentChange("", '{"root":{"children":[]}}', false);
		});
		expect(localStorage.getItem(expectedKey)).toBeNull();

		unmount();
	});

	it("carries a draft typed during loading into the seed for the loaded editor", () => {
		const { result, unmount } = renderEditing();

		// handleContentChange persists but does not advance the seed.
		const editorState =
			'{"root":{"children":[{"text":"typed while loading"}]}}';
		act(() => {
			result.current.handleLoadingDraftChange(
				"typed while loading",
				editorState,
				false,
			);
		});

		expect(localStorage.getItem(expectedKey)).toBe(editorState);
		expect(result.current.editorInitialValue).toBe("typed while loading");
		expect(result.current.initialEditorState).toBe(editorState);

		unmount();
	});

	it("loads edit text into the composer and restores the prior draft on cancel without refocusing", () => {
		const { result, unmount, beginEdit } = renderEditing();

		// Simulate the user typing a draft via handleContentChange.
		act(() => {
			result.current.handleContentChange(
				"work in progress",
				"work in progress",
				false,
			);
		});

		const remountKeyBefore = result.current.remountKey;

		act(() => {
			beginEdit({ kind: "history", id: 7 }, "edited message");
		});

		expect(result.current.editingTarget).toEqual({ kind: "history", id: 7 });
		expect(result.current.editorInitialValue).toBe("edited message");
		expect(result.current.remountKey).toBe(remountKeyBefore + 1);

		const remountKeyAfterEdit = result.current.remountKey;

		act(() => {
			result.current.handleCancelEdit();
		});

		expect(result.current.editingTarget).toBeNull();
		expect(result.current.editorInitialValue).toBe("work in progress");
		expect(result.current.remountKey).toBe(remountKeyAfterEdit + 1);
		unmount();
	});

	it("does not force focus when replacing input values on mobile", () => {
		setMobileViewport(true);
		const { result, unmount, beginEdit } = renderEditing();
		const mockInput = createMockChatInputHandle("draft before edit");
		result.current.chatInputRef.current = mockInput.handle;

		// Edit/cancel now drive the editor via editorInitialValue +
		// remountKey, so focus is never called on the mock during
		// edit and cancel flows. handleSendFromInput is the only
		// path that calls focus and it skips on mobile viewports.
		act(() => {
			beginEdit({ kind: "history", id: 7 }, "edited message");
		});
		expect(mockInput.focus).not.toHaveBeenCalled();

		act(() => {
			result.current.handleCancelEdit();
		});
		expect(mockInput.focus).not.toHaveBeenCalled();
		unmount();
	});

	it("falls back to the persisted draft when history edit starts before hydration", () => {
		localStorage.setItem(expectedKey, "persisted draft");
		const { result, unmount, beginEdit } = renderEditing();

		act(() => {
			beginEdit({ kind: "history", id: 7 }, "edited message");
		});

		act(() => {
			result.current.handleCancelEdit();
		});

		// The hook reads the persisted draft from localStorage when
		// inputValueRef hasn't been updated by handleContentChange yet.
		expect(result.current.editorInitialValue).toBe("persisted draft");
		unmount();
	});

	it("prefers the live editor value over stale persisted draft state", () => {
		localStorage.setItem(expectedKey, "stale persisted draft");
		const { result, unmount, beginEdit } = renderEditing();

		// Simulate the editor emitting a content change, which updates
		// inputValueRef to the live value.
		act(() => {
			result.current.handleContentChange("live draft", "live draft", false);
		});

		act(() => {
			beginEdit({ kind: "history", id: 7 }, "edited message");
		});

		act(() => {
			result.current.handleCancelEdit();
		});

		expect(result.current.editorInitialValue).toBe("live draft");
		unmount();
	});

	it("can load the same edit text again after send", async () => {
		const { result, onSend, unmount, beginEdit } = renderEditing();
		const mockInput = createMockChatInputHandle();
		result.current.chatInputRef.current = mockInput.handle;

		const remountKeyBefore = result.current.remountKey;

		act(() => {
			beginEdit({ kind: "history", id: 7 }, "hello");
		});

		expect(result.current.remountKey).toBe(remountKeyBefore + 1);

		await act(async () => {
			await result.current.handleSendFromInput("hello");
		});

		const remountKeyAfterSend = result.current.remountKey;

		act(() => {
			beginEdit({ kind: "history", id: 7 }, "hello");
		});

		// remountKey increments each time an edit is loaded, even for
		// the same text, so the editor is forced to reinitialize.
		expect(result.current.remountKey).toBe(remountKeyAfterSend + 1);
		expect(result.current.editorInitialValue).toBe("hello");
		expect(onSend).toHaveBeenCalledWith("hello", undefined, {
			kind: "history",
			id: 7,
		});
		unmount();
	});

	it("forwards pending attachments through history-edit send", async () => {
		const { result, onSend, unmount, beginEdit } = renderEditing();
		const attachments: PendingAttachment[] = [
			{ fileId: "file-1", mediaType: "image/png" },
		];

		act(() => {
			beginEdit({ kind: "history", id: 7 }, "hello");
		});

		await act(async () => {
			await result.current.handleSendFromInput("hello", attachments);
		});

		expect(onSend).toHaveBeenCalledWith("hello", attachments, {
			kind: "history",
			id: 7,
		});
		unmount();
	});

	it("restores the edit draft and file-block seed when an edit submission fails", async () => {
		const { result, onSend, unmount, beginEdit } = renderEditing();
		const mockInput = createMockChatInputHandle("edited message");
		const fileBlocks = [
			{ type: "file", file_id: "file-1", media_type: "image/png" },
		] as const;
		result.current.chatInputRef.current = mockInput.handle;
		onSend.mockRejectedValueOnce(new Error("boom"));
		const editorState = JSON.stringify({
			root: {
				children: [
					{
						children: [{ text: "edited message" }],
						type: "paragraph",
					},
				],
				type: "root",
			},
		});

		act(() => {
			beginEdit({ kind: "history", id: 7 }, "edited message", fileBlocks);
		});
		// The remounted editor echoes the loaded text with its serialized state.
		act(() => {
			result.current.handleContentChange("edited message", editorState, false);
		});

		await act(async () => {
			await expect(
				result.current.handleSendFromInput("edited message"),
			).rejects.toThrow("boom");
		});

		expect(mockInput.clear).toHaveBeenCalled();
		expect(result.current.inputValueRef.current).toBe("edited message");
		expect(result.current.editingTarget).toEqual({ kind: "history", id: 7 });
		expect(result.current.editingFileBlocks).toEqual(fileBlocks);
		expect(result.current.editorInitialValue).toBe("edited message");
		expect(result.current.initialEditorState).toBe(editorState);
		unmount();
	});

	it("preserves the composer and draft when send fails", async () => {
		const { result, onSend, unmount } = renderEditing();
		const mockInput = createMockChatInputHandle("hello");
		result.current.chatInputRef.current = mockInput.handle;
		onSend.mockRejectedValueOnce(new Error("boom"));

		act(() => {
			result.current.handleContentChange("hello", "hello", false);
		});

		await act(async () => {
			await expect(result.current.handleSendFromInput("hello")).rejects.toThrow(
				"boom",
			);
		});

		expect(mockInput.clear).not.toHaveBeenCalled();
		expect(mockInput.focus).not.toHaveBeenCalled();
		expect(result.current.inputValueRef.current).toBe("hello");
		expect(localStorage.getItem(expectedKey)).toBe("hello");
		unmount();
	});

	it("clears the composer and persisted draft after a successful send", async () => {
		localStorage.setItem(expectedKey, "draft to clear");
		const { result, onSend, unmount } = renderEditing();
		const mockInput = createMockChatInputHandle("hello");
		result.current.chatInputRef.current = mockInput.handle;

		await act(async () => {
			await result.current.handleSendFromInput("hello");
		});

		expect(onSend).toHaveBeenCalledWith("hello", undefined, undefined);
		expect(mockInput.clear).toHaveBeenCalled();
		expect(mockInput.focus).toHaveBeenCalled();
		expect(localStorage.getItem(expectedKey)).toBeNull();
		unmount();
	});

	it("does not write a draft key when chatID is undefined", () => {
		const { result, unmount } = renderEditing(undefined);

		act(() => {
			result.current.handleContentChange("should not persist", "{}", false);
		});

		// The ref is still updated even without persistence.
		expect(result.current.inputValueRef.current).toBe("should not persist");
		// No draft for "undefined" chatID should appear.
		expect(
			localStorage.getItem(`${draftInputStorageKeyPrefix}undefined`),
		).toBeNull();
		unmount();
	});

	it("calls focus on the input ref after a successful send", async () => {
		const { result, onSend, unmount } = renderEditing();

		// Attach a mock ChatMessageInputRef to the chatInputRef
		const mockFocus = vi.fn();
		const mockClear = vi.fn();
		const mockInputRef = {
			focus: mockFocus,
			clear: mockClear,
			setValue: vi.fn(),
			insertText: vi.fn(),
			getValue: vi.fn().mockReturnValue(""),
			addFileReference: vi.fn(),
			getContentParts: vi.fn().mockReturnValue([]),
		}; // The hook exposes chatInputRef, so assign the mock to it.
		result.current.chatInputRef.current = mockInputRef;

		await act(async () => {
			result.current.handleSendFromInput("hello");
			await vi.waitFor(() => {
				expect(onSend).toHaveBeenCalledWith("hello", undefined, undefined);
			});
		});

		expect(mockClear).toHaveBeenCalled();
		expect(mockFocus).toHaveBeenCalled();
		unmount();
	});

	it("initializes with the correct draft for each chatID", () => {
		const chatA = "chat-aaa";
		const chatB = "chat-bbb";
		localStorage.setItem(`${draftInputStorageKeyPrefix}${chatA}`, "draft A");
		localStorage.setItem(`${draftInputStorageKeyPrefix}${chatB}`, "draft B");

		// Each chatID should initialize with its own draft. This is
		// what the key={agentId} wrapper guarantees at the component
		// level (a new chatID means a full remount).
		const hookA = renderEditing(chatA);
		expect(hookA.result.current.editorInitialValue).toBe("draft A");
		hookA.unmount();

		const hookB = renderEditing(chatB);
		expect(hookB.result.current.editorInitialValue).toBe("draft B");
		hookB.unmount();
	});

	it("clears the draft from localStorage on successful send", async () => {
		localStorage.setItem(expectedKey, "draft to clear");

		const { result, unmount } = renderEditing();

		expect(localStorage.getItem(expectedKey)).toBe("draft to clear");

		await act(async () => {
			result.current.handleSendFromInput("hello");
			await vi.waitFor(() => {
				expect(localStorage.getItem(expectedKey)).toBeNull();
			});
		});
		unmount();
	});

	it("persists serialized editor state when provided", () => {
		const { result, unmount } = renderEditing();
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

		// The serialized editor state should be stored, not the plain text.
		expect(localStorage.getItem(expectedKey)).toBe(editorState);
		expect(result.current.inputValueRef.current).toBe("review this");
		unmount();
	});

	it("restores editorInitialState from a Lexical JSON draft", () => {
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
		localStorage.setItem(expectedKey, editorState);

		const { result, unmount } = renderEditing();

		expect(result.current.initialEditorState).toBe(editorState);
		expect(result.current.editorInitialValue).toBe("hello");
		unmount();
	});

	it("falls back to plain text for legacy drafts", () => {
		localStorage.setItem(expectedKey, "legacy plain text");

		const { result, unmount } = renderEditing();

		expect(result.current.initialEditorState).toBeUndefined();
		expect(result.current.editorInitialValue).toBe("legacy plain text");
		unmount();
	});

	it("persists file-reference-only drafts (no text content)", () => {
		const { result, unmount } = renderEditing();
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
			// Empty text but hasFileReferences=true should still persist.
			result.current.handleContentChange("", editorState, true);
		});

		expect(localStorage.getItem(expectedKey)).toBe(editorState);
		unmount();
	});

	it("removes draft for whitespace-only content without file references", () => {
		localStorage.setItem(expectedKey, "old draft");
		const { result, unmount } = renderEditing();

		act(() => {
			result.current.handleContentChange("   ", '{"root":{}}', false);
		});

		expect(localStorage.getItem(expectedKey)).toBeNull();
		unmount();
	});

	it("preserves serialized editor state across history edit then cancel", () => {
		const editorState = JSON.stringify({
			root: {
				children: [
					{
						children: [
							{ text: "my draft", type: "text" },
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
		localStorage.setItem(expectedKey, editorState);

		const { result, unmount, beginEdit } = renderEditing();

		expect(result.current.initialEditorState).toBe(editorState);
		expect(result.current.editorInitialValue).toBe("my draft");

		// Simulate typing so localStorage reflects the current draft.
		act(() => {
			result.current.handleContentChange("my draft", editorState, true);
		});

		// Start editing a history message.
		act(() => {
			beginEdit({ kind: "history", id: 42 }, "old message text");
		});

		expect(result.current.editingTarget).toEqual({ kind: "history", id: 42 });
		expect(result.current.initialEditorState).toBeUndefined();
		expect(result.current.editorInitialValue).toBe("old message text");

		// Cancel should restore both plain text and serialized state.
		act(() => {
			result.current.handleCancelEdit();
		});

		expect(result.current.editingTarget).toBeNull();
		expect(result.current.initialEditorState).toBe(editorState);
		expect(result.current.editorInitialValue).toBe("my draft");
		unmount();
	});

	it("returns undefined initialEditorState after edit then cancel with plain-text draft", () => {
		localStorage.setItem(expectedKey, "plain text draft");

		const { result, unmount, beginEdit } = renderEditing();

		expect(result.current.initialEditorState).toBeUndefined();

		act(() => {
			result.current.handleContentChange(
				"plain text draft",
				"plain text draft",
				false,
			);
		});

		act(() => {
			beginEdit({ kind: "history", id: 1 }, "editing");
		});

		act(() => {
			result.current.handleCancelEdit();
		});

		expect(result.current.initialEditorState).toBeUndefined();
		expect(result.current.editorInitialValue).toBe("plain text draft");
		unmount();
	});

	it("does not overwrite the persisted draft while editing a message", () => {
		const { result, unmount, beginEdit } = renderEditing();

		act(() => {
			result.current.handleContentChange("draft", "draft", false);
			beginEdit({ kind: "history", id: 7 }, "old text");
		});
		act(() => {
			result.current.handleContentChange("edited text", "edited text", false);
		});

		expect(localStorage.getItem(expectedKey)).toBe("draft");
		unmount();
	});

	describe("queued message editing", () => {
		it("saving a queued row passes the queued target and restores the draft from before the edit", async () => {
			localStorage.setItem(expectedKey, "draft");
			const { result, onSend, unmount, beginEdit } = renderEditing();
			const mockInput = createMockChatInputHandle("queued edit");
			result.current.chatInputRef.current = mockInput.handle;
			const attachments: PendingAttachment[] = [
				{ fileId: "file-1", mediaType: "image/png" },
			];

			act(() => {
				result.current.handleContentChange("draft", "draft", false);
				beginEdit({ kind: "queued", id: 42 }, "queued text");
			});

			await act(async () => {
				await result.current.handleSendFromInput("queued edit", attachments);
			});

			expect(onSend).toHaveBeenCalledWith("queued edit", attachments, {
				kind: "queued",
				id: 42,
			});
			expect(mockInput.clear).toHaveBeenCalled();
			expect(mockInput.focus).toHaveBeenCalled();
			expect(result.current.editingTarget).toBeNull();
			expect(result.current.editingFileBlocks).toEqual([]);
			expect(result.current.editorInitialValue).toBe("draft");
			expect(result.current.inputValueRef.current).toBe("draft");
			expect(localStorage.getItem(expectedKey)).toBe("draft");
			unmount();
		});

		// The row's text is "queued text"; the draft before the edit is "draft".
		it.each([
			[
				"modified text stays as a new-message draft",
				"queued edit",
				"queued edit",
				"draft",
			],
			[
				"unmodified text gives way to the draft from before the edit and the composer counts as untouched",
				"queued text",
				"draft",
				undefined,
			],
		])(
			"when the server ends the edit and marks no other row, %s",
			(_name, textWhileEditing, expectedDraft, expectedMode) => {
				const { result, unmount, beginEdit, markOnServer } = renderEditing();

				act(() => {
					result.current.handleContentChange("draft", "draft", false);
					beginEdit({ kind: "queued", id: 42 }, "queued text");
				});
				act(() => {
					result.current.handleContentChange(
						textWhileEditing,
						textWhileEditing,
						false,
					);
				});

				markOnServer(null);

				expect(result.current.editingTarget).toBeNull();
				expect(result.current.composerMode).toBe(expectedMode);
				expect(result.current.inputValueRef.current).toBe(expectedDraft);
				expect(localStorage.getItem(expectedKey)).toBe(expectedDraft);
				unmount();
			},
		);

		it.each([
			[
				"modified text stays as a new-message draft and no edit opens",
				"queued edit",
				null,
				"queued edit",
			],
			[
				"unmodified text follows the edit to the newly marked row",
				"queued text",
				{ kind: "queued", id: 6 },
				"other row",
			],
		])(
			"when another client moves the edit to a different row, %s",
			(_name, textWhileEditing, expectedTarget, expectedText) => {
				const { result, unmount, beginEdit, markOnServer } = renderEditing();

				act(() => {
					result.current.handleContentChange("draft", "draft", false);
					beginEdit({ kind: "queued", id: 42 }, "queued text");
				});
				act(() => {
					result.current.handleContentChange(
						textWhileEditing,
						textWhileEditing,
						false,
					);
				});

				markOnServer(6, "other row");

				expect(result.current.editingTarget).toEqual(expectedTarget);
				expect(result.current.inputValueRef.current).toBe(expectedText);
				unmount();
			},
		);

		it("on reload, opens the marked row and keeps the saved draft for when the edit ends", () => {
			localStorage.setItem(expectedKey, "saved draft");
			const { result, unmount, markOnServer } = renderEditing();
			expect(result.current.editingTarget).toBeNull();

			// Reload: the store hydrates after the first render.
			markOnServer(5, "run the migrations");

			expect(result.current.editingTarget).toEqual({ kind: "queued", id: 5 });
			expect(result.current.editorInitialValue).toBe("run the migrations");
			expect(result.current.composerMode).toBeUndefined();

			// The seed echo does not count as input; the composer stays untouched.
			act(() => {
				result.current.handleContentChange(
					"run the migrations",
					"run the migrations",
					false,
				);
			});
			expect(result.current.composerMode).toBeUndefined();

			markOnServer(null);
			expect(result.current.editingTarget).toBeNull();
			expect(result.current.editorInitialValue).toBe("saved draft");
			unmount();
		});

		it("the first keystroke pins the edit to the marked row, so a later marker on another row does not open it", () => {
			const { result, unmount, markOnServer } = renderEditing();
			markOnServer(5);

			act(() => {
				result.current.handleContentChange(
					"queued text!",
					"queued text!",
					false,
				);
			});
			expect(result.current.editingTarget).toEqual({ kind: "queued", id: 5 });

			markOnServer(null);
			expect(result.current.editingTarget).toBeNull();
			expect(result.current.inputValueRef.current).toBe("queued text!");

			markOnServer(6);
			expect(result.current.editingTarget).toBeNull();
			expect(result.current.inputValueRef.current).toBe("queued text!");
			unmount();
		});

		it("typing into an untouched composer keeps it on the draft when a row is marked later", () => {
			const { result, unmount, markOnServer } = renderEditing();
			act(() => {
				result.current.handleContentChange("hi", "hi", false);
			});

			markOnServer(5);
			expect(result.current.editingTarget).toBeNull();
			expect(result.current.inputValueRef.current).toBe("hi");
			unmount();
		});

		it("Cancel restores the draft from before the edit and leaves the composer in draft mode", () => {
			const { result, unmount, beginEdit } = renderEditing();
			act(() => {
				result.current.handleContentChange("draft", "draft", false);
				beginEdit({ kind: "queued", id: 42 }, "queued text");
			});
			act(() => {
				result.current.handleContentChange("queued edit", "queued edit", false);
			});

			act(() => {
				result.current.handleCancelEdit();
			});
			expect(result.current.composerMode).toBe("draft");
			expect(result.current.editingTarget).toBeNull();
			expect(result.current.editorInitialValue).toBe("draft");
			expect(result.current.inputValueRef.current).toBe("draft");
			unmount();
		});
	});
});
