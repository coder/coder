import { useLayoutEffect, useRef, useState } from "react";
import type { ChatMessagePart } from "#/api/typesGenerated";
import { isMobileViewport } from "#/utils/mobile";
import type { ChatMessageInputRef } from "../components/AgentChatInput";
import type { EditingTarget } from "../components/ChatConversation/types";
import type { PendingAttachment } from "../components/ChatPageContent";
import {
	draftInputStorageKeyPrefix,
	type ParsedDraft,
	parseStoredDraft,
} from "../utils/draftStorage";

export class BuiltInCommandPendingError extends Error {}

/** @internal Exported for testing. */
export function useConversationEditingState(deps: {
	chatID: string | undefined;
	onSend: (
		message: string,
		attachments?: readonly PendingAttachment[],
		editingTarget?: EditingTarget,
	) => Promise<void>;
	chatInputRef: React.RefObject<ChatMessageInputRef | null>;
	inputValueRef: React.RefObject<string>;
}) {
	const { chatID, onSend, chatInputRef, inputValueRef } = deps;
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

	const [editingTarget, setEditingTarget] = useState<EditingTarget | null>(
		null,
	);
	const [draftBeforeEdit, setDraftBeforeEdit] = useState<ParsedDraft | null>(
		null,
	);
	const [editingFileBlocks, setEditingFileBlocks] = useState<
		readonly ChatMessagePart[]
	>([]);
	const editingMessageId =
		editingTarget?.kind === "history" ? editingTarget.id : null;

	const handleBeginEdit = (
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
			setDraftBeforeEdit({
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

	const restoreDraftBeforeEdit = () => {
		const savedText = draftBeforeEdit?.text ?? "";
		const savedState = draftBeforeEdit?.editorState;
		setDraftState({
			editorInitialValue: savedText,
			initialEditorState: savedState,
		});
		serializedEditorStateRef.current = savedState;
		setRemountKey((k) => k + 1);
		inputValueRef.current = savedText;
		setEditingTarget(null);
		setDraftBeforeEdit(null);
		setEditingFileBlocks([]);
	};

	// Leaves edit mode with the composer text kept as the draft.
	const leaveEdit = () => {
		setEditingTarget(null);
		setDraftBeforeEdit(null);
		setEditingFileBlocks([]);
		if (draftStorageKey) {
			const draft = serializedEditorStateRef.current ?? inputValueRef.current;
			if (inputValueRef.current.trim()) {
				try {
					localStorage.setItem(draftStorageKey, draft);
				} catch {
					// QuotaExceededError, silently discard the draft.
				}
			} else {
				localStorage.removeItem(draftStorageKey);
			}
		}
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
			setDraftBeforeEdit(null);
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
			// A saved queued row does not start a turn; the pre-edit draft
			// is restored.
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
		handleBeginEdit,
		handleCancelEdit: restoreDraftBeforeEdit,
		leaveEdit,
		handleSendFromInput,
		handleContentChange,
		handleLoadingDraftChange,
	};
}
