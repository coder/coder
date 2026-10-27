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
// those of after. Returns undefined when either state is missing or
// unparsable; the editor then seeds the plain text.
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

type SendInFlight = {
	message: string;
	editorState: string | undefined;
	fileBlocks: readonly ChatMessagePart[];
	editedMessageId: number | null;
};

type ComposerContent = {
	text: string;
	editorState: string | undefined;
	// File-reference and skill chips have no plain text.
	hasFileReferences: boolean;
};

const hasContent = (content: ComposerContent) =>
	Boolean(content.text.trim()) || content.hasFileReferences;

// The composer content a failed send restores: the sent message, followed by
// what was typed during the request.
const restoredDraft = (send: SendInFlight, typed: ComposerContent) => {
	if (!hasContent(typed)) {
		return { text: send.message, editorState: send.editorState };
	}
	return {
		text: `${send.message}\n\n${typed.text}`,
		editorState: joinEditorStates(send.editorState, typed.editorState),
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
	const hasFileReferencesRef = useRef(false);
	const composerContent = (): ComposerContent => ({
		text: inputValueRef.current,
		editorState: serializedEditorStateRef.current,
		hasFileReferences: hasFileReferencesRef.current,
	});
	// The send in flight, from a submit's clear until it settles. While it is
	// set, content changes save what a failure would restore as the draft, so
	// a reload or unmount mid-send keeps the sent message and what was typed
	// since. A history edit leaves the draft from before the edit started.
	const sendInFlightRef = useRef<SendInFlight | null>(null);

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
		// The rollback and draft write of a send in flight would overwrite the
		// edit, so Edit does nothing until the send settles.
		if (sendInFlightRef.current) {
			return;
		}
		if (editingMessageId === null) {
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

	// Returns a rollback for a failed send: the message, then anything typed
	// during the request. An edit comes back alone, because the composer showed
	// the compose draft during it, and the typed text stays that draft.
	const clearInputForSend = (message: string) => {
		const snapshot: SendInFlight = {
			message,
			editorState: serializedEditorStateRef.current,
			fileBlocks: editingFileBlocks,
			editedMessageId: editingMessageId,
		};

		sendInFlightRef.current = snapshot;
		chatInputRef.current?.clear();
		inputValueRef.current = "";
		hasFileReferencesRef.current = false;
		setEditingMessageId(null);

		return () => {
			const typed = composerContent();
			const isEdit = snapshot.editedMessageId !== null;
			const { text, editorState } = isEdit
				? { text: snapshot.message, editorState: snapshot.editorState }
				: restoredDraft(snapshot, typed);
			if (isEdit && hasContent(typed)) {
				setDraftBeforeHistoryEdit({
					text: typed.text,
					editorState: typed.editorState,
				});
			}
			setDraftState({
				editorInitialValue: text,
				initialEditorState: editorState,
			});
			serializedEditorStateRef.current = editorState;
			setRemountKey((k) => k + 1);
			inputValueRef.current = text;
			setEditingMessageId(snapshot.editedMessageId);
			setEditingFileBlocks(snapshot.fileBlocks);
		};
	};

	// The composer was cleared at submit, so after a successful send it holds
	// only what was typed during the request, which is the chat's draft now.
	const finalizeSuccessfulSend = (editedMessageID: number | undefined) => {
		const draft = serializedEditorStateRef.current;
		if (hasContent(composerContent()) && draft) {
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
			sendInFlightRef.current = null;
			if (error instanceof BuiltInCommandPendingError) {
				return;
			}
			throw error;
		}
		sendInFlightRef.current = null;

		finalizeSuccessfulSend(target);
	};

	const handleContentChange = (
		content: string,
		serializedEditorState: string,
		hasFileReferences: boolean,
	) => {
		inputValueRef.current = content;
		serializedEditorStateRef.current = serializedEditorState;
		hasFileReferencesRef.current = hasFileReferences;

		// Don't overwrite the persisted draft while editing a message.
		// The original draft is saved in React state and should survive a cancel.
		if (editingTarget !== null) {
			return;
		}
		const sendInFlight = sendInFlightRef.current;
		if (sendInFlight) {
			if (sendInFlight.editedMessageId !== null) {
				return;
			}
			const draft = restoredDraft(sendInFlight, composerContent());
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

		if (hasContent(composerContent())) {
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
