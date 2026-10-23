import { useLayoutEffect, useRef, useState } from "react";
import type { ChatMessagePart } from "#/api/typesGenerated";
import type { ChatMessageInputRef } from "../components/AgentChatInput";
import type { EditingTarget } from "../components/ChatConversation/types";
import type { SendChatMessageOptions } from "../components/ChatPageContent";
import {
	draftInputStorageKeyPrefix,
	type ParsedDraft,
	parseStoredDraft,
} from "../utils/draftStorage";

export class BuiltInCommandPendingError extends Error {}

export type SendChatTurnOptions = SendChatMessageOptions & {
	editingTarget?: EditingTarget;
};

// Joins two serialized Lexical states, the blocks of before followed by
// those of after. Returns undefined when either cannot be parsed; the editor
// then seeds the plain text instead.
const joinEditorStates = (
	before: string | undefined,
	after: string | undefined,
): string | undefined => {
	if (!before || !after) {
		return undefined;
	}
	try {
		const head = JSON.parse(before);
		const tail = JSON.parse(after);
		return JSON.stringify({
			...head,
			root: {
				...head.root,
				children: [...head.root.children, ...tail.root.children],
			},
		});
	} catch {
		return undefined;
	}
};

type PendingSend = {
	message: string;
	editorState: string | undefined;
	fileBlocks: readonly ChatMessagePart[];
	messageId: number | null;
};

// The composer content a failed send restores: the sent message, followed by
// what was typed during the request.
const restoredDraft = (
	send: PendingSend,
	typedText: string,
	typedEditorState: string | undefined,
) => {
	if (!typedText.trim()) {
		return { text: send.message, editorState: send.editorState };
	}
	return {
		text: `${send.message}\n\n${typedText}`,
		editorState: joinEditorStates(send.editorState, typedEditorState),
	};
};

/** @internal Exported for testing. */
export function useConversationEditingState(deps: {
	chatID: string;
	onSend: (options: SendChatTurnOptions) => Promise<void>;
	chatInputRef: React.RefObject<ChatMessageInputRef | null>;
	inputValueRef: React.RefObject<string>;
}) {
	const { chatID, onSend, chatInputRef, inputValueRef } = deps;
	const draftStorageKey = `${draftInputStorageKeyPrefix}${chatID}`;
	const [{ editorInitialValue, initialEditorState }, setDraftState] = useState(
		() => {
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
	// The send in flight, from a submit's clear until it settles. While it is
	// set, content changes save what a failure would restore as the draft, so
	// a reload or unmount mid-send keeps the sent message and what was typed
	// since. A history edit leaves the draft from before the edit started.
	const pendingSendRef = useRef<PendingSend | null>(null);

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

	const handleBeginEdit = (
		target: EditingTarget,
		text: string,
		fileBlocks?: readonly ChatMessagePart[],
		editorState?: string,
	) => {
		if (editingTarget === null) {
			// Read the current serialized editor state from localStorage
			// (kept up-to-date by handleContentChange) rather than from
			// the stale initialEditorState React state.
			const currentEditorState = parseStoredDraft(
				localStorage.getItem(draftStorageKey),
			).editorState;
			setDraftBeforeEdit({
				text: inputValueRef.current,
				editorState: currentEditorState,
			});
		}
		setEditingTarget(target);
		setDraftState({
			editorInitialValue: text,
			initialEditorState: editorState,
		});
		serializedEditorStateRef.current = editorState;
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

	// Returns a rollback for a failed send. The composer stays editable during
	// the request, so the rollback puts the message before anything typed since.
	const clearInputForSend = (message: string) => {
		const snapshot: PendingSend = {
			message,
			editorState: serializedEditorStateRef.current,
			fileBlocks: editingFileBlocks,
			target: editingTarget,
		};

		pendingSendRef.current = snapshot;
		chatInputRef.current?.clear();
		inputValueRef.current = "";
		setEditingTarget(null);

		return () => {
			const { text, editorState } = restoredDraft(
				snapshot,
				inputValueRef.current,
				serializedEditorStateRef.current,
			);
			setDraftState({
				editorInitialValue: text,
				initialEditorState: editorState,
			});
			serializedEditorStateRef.current = editorState;
			setRemountKey((k) => k + 1);
			inputValueRef.current = text;
			setEditingMessageId(snapshot.messageId);
			setEditingFileBlocks(snapshot.fileBlocks);
		};
	};

	// Ends a successful send. The composer was cleared at submit, so any
	// content in it was typed during the request. That content stays and
	// becomes the saved draft.
	const finalizeSuccessfulSend = (editedMessageID: number | undefined) => {
		const draft = serializedEditorStateRef.current;
		if (inputValueRef.current.trim() && draft) {
			try {
				localStorage.setItem(draftStorageKey, draft);
			} catch {
				// QuotaExceededError, silently discard the draft.
			}
		} else {
			localStorage.removeItem(draftStorageKey);
		}
		if (editedMessageID !== undefined) {
			setDraftBeforeHistoryEdit(null);
			setEditingFileBlocks([]);
		}
	};

	// Wraps the parent onSend to clear local input/editing state.
	const handleSendFromInput = async ({
		message,
		attachments,
		workspaceUploads,
	}: SendChatMessageOptions) => {
		const target = editingTarget ?? undefined;
		const sendPromise = onSend({
			message,
			attachments,
			workspaceUploads,
			editingTarget: target,
		});

		const rollback = clearInputForSend(message);

		try {
			await sendPromise;
		} catch (error) {
			rollback();
			pendingSendRef.current = null;
			if (error instanceof BuiltInCommandPendingError) {
				return;
			}
			throw error;
		}
		pendingSendRef.current = null;

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
		const pendingSend = pendingSendRef.current;
		if (pendingSend) {
			if (pendingSend.messageId !== null) {
				return;
			}
			const draft = restoredDraft(pendingSend, content, serializedEditorState);
			const saved = draft.editorState ?? draft.text;
			if (!saved) {
				localStorage.removeItem(draftStorageKey);
				return;
			}
			try {
				localStorage.setItem(draftStorageKey, saved);
			} catch {
				// QuotaExceededError, silently discard the draft.
			}
			return;
		}

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
		editingFileBlocks,
		handleBeginEdit,
		handleCancelEdit: restoreDraftBeforeEdit,
		handleSendFromInput,
		handleContentChange,
		handleLoadingDraftChange,
	};
}
