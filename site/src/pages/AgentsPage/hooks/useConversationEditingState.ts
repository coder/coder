import { useEffect, useLayoutEffect, useRef, useState } from "react";
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

type RequestInFlight = {
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
const restoredDraft = (send: RequestInFlight, typed: ComposerContent) => {
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
	// The request in flight, from submit until it settles or the page unmounts.
	// While a new message is in flight, content changes save what a failure
	// would restore as the draft, so a reload or unmount mid-send keeps the
	// sent message and what was typed since.
	const requestInFlightRef = useRef<RequestInFlight | null>(null);
	// The draft this hook last stored, or null if it stored none or last
	// removed it.
	const storedDraftRef = useRef<string | null>(null);
	useEffect(
		() => () => {
			requestInFlightRef.current = null;
		},
		[],
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

	// Seeds a remounted composer with text, or with editorState when set.
	const showInComposer = (text: string, editorState: string | undefined) => {
		setDraftState({
			editorInitialValue: text,
			initialEditorState: editorState,
		});
		serializedEditorStateRef.current = editorState;
		setRemountKey((k) => k + 1);
		inputValueRef.current = text;
	};

	const handleEditUserMessage = (
		messageId: number,
		text: string,
		fileBlocks?: readonly ChatMessagePart[],
		editorState?: string,
	) => {
		// The rollback and draft write of a request in flight would overwrite
		// the edit, so Edit does nothing until the request settles.
		if (requestInFlightRef.current) {
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
		setEditingMessageId(messageId);
		showInComposer(text, editorState);
		setEditingFileBlocks(fileBlocks ?? []);
	};

	const showComposeDraft = () =>
		showInComposer(
			draftBeforeHistoryEdit?.text ?? "",
			draftBeforeHistoryEdit?.editorState,
		);

	const handleCancelHistoryEdit = () => {
		showComposeDraft();
		setEditingMessageId(null);
		setDraftBeforeHistoryEdit(null);
		setEditingFileBlocks([]);
	};

	// Returns a rollback for a failed request. An edit comes back alone, and
	// what the composer showed during it becomes the compose draft that Cancel
	// restores.
	const removeSubmittedContent = (message: string) => {
		const snapshot: RequestInFlight = {
			message,
			editorState: serializedEditorStateRef.current,
			fileBlocks: editingFileBlocks,
			editedMessageId: editingMessageId,
		};

		requestInFlightRef.current = snapshot;
		if (snapshot.editedMessageId === null) {
			chatInputRef.current?.clear();
			inputValueRef.current = "";
		} else {
			showComposeDraft();
		}
		hasFileReferencesRef.current = false;
		setEditingMessageId(null);

		return () => {
			const shown = composerContent();
			const isEdit = snapshot.editedMessageId !== null;
			const { text, editorState } = isEdit
				? { text: snapshot.message, editorState: snapshot.editorState }
				: restoredDraft(snapshot, shown);
			if (isEdit) {
				setDraftBeforeHistoryEdit({
					text: shown.text,
					editorState: shown.editorState,
				});
			}
			showInComposer(text, editorState);
			setEditingMessageId(snapshot.editedMessageId);
			setEditingFileBlocks(snapshot.fileBlocks);
		};
	};

	// Also records the stored draft in storedDraftRef for the check after
	// unmount in handleSendFromInput.
	const saveDraft = (draft: string | undefined) => {
		if (!draft) {
			localStorage.removeItem(draftStorageKey);
			storedDraftRef.current = null;
			return;
		}
		try {
			localStorage.setItem(draftStorageKey, draft);
			storedDraftRef.current = draft;
		} catch {
			// QuotaExceededError, silently discard the draft.
		}
	};

	// After a successful request the composer shows the chat's draft: for a
	// send what was typed since, for an edit the compose draft as edited since.
	const finalizeSuccessfulSend = (editedMessageID: number | undefined) => {
		saveDraft(
			hasContent(composerContent())
				? serializedEditorStateRef.current
				: undefined,
		);
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

		const rollback = removeSubmittedContent(message);

		try {
			await sendPromise;
		} catch (error) {
			rollback();
			requestInFlightRef.current = null;
			if (error instanceof BuiltInCommandPendingError) {
				return;
			}
			throw error;
		}
		// The unmount cleanup sets the ref to null. Then a success writes the
		// draft only if storage still holds what this hook stored, not a later
		// visit's draft.
		const visitEnded = requestInFlightRef.current === null;
		requestInFlightRef.current = null;
		if (
			visitEnded &&
			localStorage.getItem(draftStorageKey) !== storedDraftRef.current
		) {
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
		hasFileReferencesRef.current = hasFileReferences;

		// Don't overwrite the persisted draft while editing a message.
		// The original draft is saved in React state and should survive a cancel.
		if (editingTarget !== null) {
			return;
		}
		const requestInFlight = requestInFlightRef.current;
		if (requestInFlight && requestInFlight.editedMessageId === null) {
			const draft = restoredDraft(requestInFlight, composerContent());
			saveDraft(draft.editorState ?? draft.text);
			return;
		}

		saveDraft(
			hasContent(composerContent()) ? serializedEditorState : undefined,
		);
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
