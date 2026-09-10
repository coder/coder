import { useLayoutEffect, useRef, useState } from "react";
import type { ChatMessagePart } from "#/api/typesGenerated";
import { isMobileViewport } from "#/utils/mobile";
import type { ChatMessageInputRef } from "../components/AgentChatInput";
import type { PendingAttachment } from "../components/ChatPageContent";
import {
	draftInputStorageKeyPrefix,
	type ParsedDraft,
	parseStoredDraft,
} from "../utils/draftStorage";

export class BuiltInCommandPendingError extends Error {}

/**
 * The message the composer is editing. History rows and queued rows have
 * independent ID spaces, so the kind is required to interpret the ID.
 */
export type EditingTarget =
	| { kind: "history"; id: number }
	| { kind: "queued"; id: number };

/** @internal Exported for testing. */
export function useConversationEditingState(deps: {
	chatID: string | undefined;
	onSend: (
		message: string,
		attachments?: readonly PendingAttachment[],
		editingTarget?: EditingTarget,
	) => Promise<void>;
	// Releases the hold on a queued row when its edit is cancelled. A
	// rejection keeps the composer in edit mode; the callback owns
	// error reporting.
	onResumeQueuedMessage: (id: number) => Promise<void>;
	chatInputRef: React.RefObject<ChatMessageInputRef | null>;
	inputValueRef: React.RefObject<string>;
}) {
	const { chatID, onSend, onResumeQueuedMessage, chatInputRef, inputValueRef } =
		deps;
	const draftStorageKey = chatID
		? `${draftInputStorageKeyPrefix}${chatID}`
		: null;
	const [{ editorInitialValue, initialEditorState }, setDraftState] = useState(
		() => {
			if (!draftStorageKey) {
				return { editorInitialValue: "", initialEditorState: undefined };
			}
			const draft = parseStoredDraft(localStorage.getItem(draftStorageKey));
			return {
				editorInitialValue: draft.text,
				initialEditorState: draft.editorState,
			};
		},
	);
	const serializedEditorStateRef = useRef<string | undefined>(
		initialEditorState,
	);

	// Monotonic counter to force LexicalComposer remount.
	const [remountKey, setRemountKey] = useState(0);

	// Sync the ref with the initial draft value so callers that
	// read inputValueRef.current see the persisted draft. Uses a
	// layout effect so the value is available before paint.
	const initialSyncDone = useRef(false);
	useLayoutEffect(() => {
		if (!initialSyncDone.current && editorInitialValue) {
			initialSyncDone.current = true;
			(inputValueRef as React.MutableRefObject<string>).current =
				editorInitialValue;
		}
	}, [editorInitialValue, inputValueRef]);

	// Editing state.
	const [editingTarget, setEditingTarget] = useState<EditingTarget | null>(
		null,
	);
	const [draftBeforeHistoryEdit, setDraftBeforeHistoryEdit] =
		useState<ParsedDraft | null>(null);
	const [editingFileBlocks, setEditingFileBlocks] = useState<
		readonly ChatMessagePart[]
	>([]);
	const editingMessageId =
		editingTarget?.kind === "history" ? editingTarget.id : null;

	const loadEditIntoComposer = (
		target: EditingTarget,
		text: string,
		fileBlocks?: readonly ChatMessagePart[],
	) => {
		if (editingTarget === null) {
			// Read the current serialized editor state from localStorage
			// (kept up-to-date by handleContentChange) rather than from
			// the stale initialEditorState React state.
			const currentEditorState = draftStorageKey
				? parseStoredDraft(localStorage.getItem(draftStorageKey)).editorState
				: undefined;
			setDraftBeforeHistoryEdit({
				text: inputValueRef.current,
				editorState: currentEditorState,
			});
		}
		setEditingTarget(target);
		setDraftState({
			editorInitialValue: text,
			initialEditorState: undefined,
		});
		serializedEditorStateRef.current = undefined;
		setRemountKey((k) => k + 1);
		inputValueRef.current = text;
		setEditingFileBlocks(fileBlocks ?? []);
	};

	const handleEditUserMessage = (
		messageId: number,
		text: string,
		fileBlocks?: readonly ChatMessagePart[],
	) => {
		loadEditIntoComposer({ kind: "history", id: messageId }, text, fileBlocks);
	};

	const handleEditQueuedMessage = (
		queuedMessageId: number,
		text: string,
		fileBlocks?: readonly ChatMessagePart[],
	) => {
		loadEditIntoComposer(
			{ kind: "queued", id: queuedMessageId },
			text,
			fileBlocks,
		);
	};

	const restoreDraftBeforeEdit = () => {
		const savedText = draftBeforeHistoryEdit?.text ?? "";
		const savedState = draftBeforeHistoryEdit?.editorState;
		setDraftState({
			editorInitialValue: savedText,
			initialEditorState: savedState,
		});
		serializedEditorStateRef.current = savedState;
		setRemountKey((k) => k + 1);
		inputValueRef.current = savedText;
		setEditingTarget(null);
		setDraftBeforeHistoryEdit(null);
		setEditingFileBlocks([]);
	};

	const handleCancelHistoryEdit = async () => {
		if (editingTarget?.kind === "queued") {
			try {
				await onResumeQueuedMessage(editingTarget.id);
			} catch {
				// The row is still held, so the edit stays open for a retry.
				return;
			}
		}
		restoreDraftBeforeEdit();
	};

	// Clears the composer for an in-flight edit and returns a rollback
	// function that restores the editing draft if the send fails.
	const clearInputForEdit = (message: string) => {
		const snapshot = {
			editorState: serializedEditorStateRef.current,
			fileBlocks: editingFileBlocks,
			target: editingTarget,
		};

		chatInputRef.current?.clear();
		inputValueRef.current = "";
		setEditingTarget(null);

		return () => {
			setDraftState({
				editorInitialValue: message,
				initialEditorState: snapshot.editorState,
			});
			serializedEditorStateRef.current = snapshot.editorState;
			setRemountKey((k) => k + 1);
			inputValueRef.current = message;
			setEditingTarget(snapshot.target);
			setEditingFileBlocks(snapshot.fileBlocks);
		};
	};

	// Clears all input and editing state after a successful send.
	const finalizeSuccessfulSend = (target: EditingTarget | undefined) => {
		chatInputRef.current?.clear();
		if (!isMobileViewport()) {
			chatInputRef.current?.focus();
		}
		inputValueRef.current = "";
		serializedEditorStateRef.current = undefined;
		if (draftStorageKey) {
			localStorage.removeItem(draftStorageKey);
		}
		if (target !== undefined) {
			setDraftBeforeHistoryEdit(null);
			setEditingFileBlocks([]);
		}
	};

	// Wraps the parent onSend to clear local input/editing state.
	const handleSendFromInput = async (
		message: string,
		attachments?: readonly PendingAttachment[],
	) => {
		const target = editingTarget ?? undefined;
		const sendPromise = onSend(message, attachments, target);

		// For edits, clear input immediately and prepare a rollback in
		// case the send fails.
		const rollback =
			target !== undefined ? clearInputForEdit(message) : undefined;

		try {
			await sendPromise;
		} catch (error) {
			if (error instanceof BuiltInCommandPendingError) {
				return;
			}
			rollback?.();
			throw error;
		}

		if (target?.kind === "queued") {
			// The queued row stays in the queue, so the draft the user
			// had before editing it is still wanted.
			restoreDraftBeforeEdit();
			if (!isMobileViewport()) {
				chatInputRef.current?.focus();
			}
			return;
		}
		finalizeSuccessfulSend(target);
	};

	const handleContentChange = (
		content: string,
		serializedEditorState: string,
		hasFileReferences: boolean,
	) => {
		inputValueRef.current = content;
		serializedEditorStateRef.current = serializedEditorState;

		// Don't overwrite the persisted draft while editing a message.
		// The original draft is saved in React state and should survive a cancel.
		if (editingTarget !== null) {
			return;
		}

		if (draftStorageKey) {
			const shouldPersist = content.trim() || hasFileReferences;
			if (shouldPersist) {
				try {
					localStorage.setItem(draftStorageKey, serializedEditorState);
				} catch {
					// QuotaExceededError, silently discard the draft.
				}
			} else {
				localStorage.removeItem(draftStorageKey);
			}
		}
	};

	// Separate from handleContentChange, which avoids setState to prevent
	// per-keystroke re-renders. The loading editor is a different instance
	// that unmounts on load, so the seed must advance here.
	const handleLoadingDraftChange = (
		content: string,
		serializedEditorState: string,
		hasFileReferences: boolean,
	) => {
		handleContentChange(content, serializedEditorState, hasFileReferences);
		setDraftState({
			editorInitialValue: content,
			initialEditorState: serializedEditorState,
		});
	};

	return {
		inputValueRef,
		chatInputRef,
		editorInitialValue,
		initialEditorState,
		remountKey,
		editingTarget,
		editingMessageId,
		editingFileBlocks,
		handleEditUserMessage,
		handleEditQueuedMessage,
		handleCancelHistoryEdit,
		handleSendFromInput,
		handleContentChange,
		handleLoadingDraftChange,
	};
}
