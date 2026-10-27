import { useEffect, useLayoutEffect, useRef, useState } from "react";
import type { ChatMessagePart } from "#/api/typesGenerated";
import type { ChatMessageInputRef } from "../components/AgentChatInput";
import { getEditableContentPayload } from "../components/ChatConversation/messageParsing";
import type { EditingTarget } from "../components/ChatConversation/types";
import type { SendChatMessageOptions } from "../components/ChatPageContent";
import {
	draftInputStorageKeyPrefix,
	getChipNodes,
	type ParsedDraft,
	parseStoredDraft,
} from "../utils/draftStorage";
import type { ComposerMode } from "./useQueuedMessageEdit";

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
	composerMode: ComposerMode;
	setComposerMode: (mode: ComposerMode) => void;
	// The row the composer edits and its content as the editor loads it.
	target: EditingTarget | null;
	targetContent: readonly ChatMessagePart[] | undefined;
	// Called when the editor loads a target or drops one because the target
	// went away. Cancel, sends and send rollbacks do not call it.
	onLoadedTargetChange?: () => void;
}) {
	const {
		chatID,
		onSend,
		chatInputRef,
		inputValueRef,
		composerMode,
		setComposerMode,
		target,
		targetContent,
		onLoadedTargetChange,
	} = deps;

	const draftStorageKey = `${draftInputStorageKeyPrefix}${chatID}`;
	// Holds the SavedQueuedEdit of an open queued edit, so typed text survives
	// a reload or a return to the chat until the edit ends. Every tab shares
	// it, and loads it only while the row still has the content it was typed
	// against.
	const queuedEditStorageKey = (id: number) =>
		`${draftStorageKey}.queued.${id}`;
	const removeQueuedEdit = (target: EditingTarget | undefined) => {
		if (target?.kind === "queued") {
			localStorage.removeItem(queuedEditStorageKey(target.id));
		}
	};

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
	// The send in flight, from submit until it settles or the page unmounts.
	// While a new message is in flight, content changes save what a failure
	// would restore as the draft, so a reload or unmount mid-send keeps the
	// sent message and what was typed since.
	const sendInFlightRef = useRef<SendInFlight | null>(null);
	// A request that settles after unmount belongs to an earlier visit, so it
	// must not overwrite the draft that a later visit saved.
	useEffect(
		() => () => {
			sendInFlightRef.current = null;
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

	const [loadedTarget, setLoadedTarget] = useState<LoadedTarget | null>(null);
	// Counts target loads, so code that resumes after an await can tell
	// whether another edit opened in the meantime.
	const targetLoadCountRef = useRef(0);
	const [draftBeforeEdit, setDraftBeforeEdit] = useState<ParsedDraft | null>(
		null,
	);

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
		hasFileReferencesRef.current = false;
		loadedContentRef.current = { text, editorState };
	};

	// The editor is plain text apart from file-reference and skill chips,
	// which have no text, so the text and the chips in order identify its
	// content; only a chip moved within unchanged text goes unseen. Whole
	// serialized states are not compared because a stored state need not
	// match how the editor serializes it once loaded.
	const isEditorChanged = () => {
		const loaded = loadedContentRef.current;
		return (
			inputValueRef.current !== loaded.text ||
			!isEqual(
				getChipNodes(serializedEditorStateRef.current),
				getChipNodes(loaded.editorState),
			)
		);
	};

	const writeDraft = (
		text: string,
		serializedEditorState: string,
		hasFileReferences: boolean,
	) => {
		if (text.trim() || hasFileReferences) {
			try {
				localStorage.setItem(draftStorageKey, serializedEditorState);
			} catch {
				// QuotaExceededError, silently discard the draft.
			}
		} else {
			localStorage.removeItem(draftStorageKey);
		}
	};

	const persistDraft = () => {
		writeDraft(
			inputValueRef.current,
			serializedEditorStateRef.current ?? inputValueRef.current,
			hasFileReferencesRef.current,
		);
	};

	const showComposeDraft = () => {
		const savedText = draftBeforeHistoryEdit?.text ?? "";
		const savedState = draftBeforeHistoryEdit?.editorState;
		setDraftState({
			editorInitialValue: savedText,
			initialEditorState: savedState,
		});
		serializedEditorStateRef.current = savedState;
		setRemountKey((k) => k + 1);
		inputValueRef.current = savedText;
	};

	const handleCancelHistoryEdit = () => {
		showComposeDraft();
		setEditingMessageId(null);
		setDraftBeforeHistoryEdit(null);
		setEditingFileBlocks([]);
	};

	// Returns a rollback for a failed request. A send comes back followed by
	// what was typed since. An edit comes back alone, and what the composer
	// showed during it becomes the compose draft that Cancel restores.
	const clearInputForSend = (message: string) => {
		const snapshot: SendInFlight = {
			message,
			editorState: serializedEditorStateRef.current,
			fileBlocks: editingFileBlocks,
			editedMessageId: editingMessageId,
		};

		sendInFlightRef.current = snapshot;
		if (snapshot.editedMessageId === null) {
			chatInputRef.current?.clear();
			inputValueRef.current = "";
		} else {
			// The edit leaves edit mode, so the composer shows the compose draft.
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

	// Saves draft as the chat's draft, or removes the saved draft if it is empty.
	const saveDraft = (draft: string | undefined) => {
		if (!draft) {
			localStorage.removeItem(draftStorageKey);
			return;
		}
		try {
			localStorage.setItem(draftStorageKey, draft);
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
		const sendTarget = target ?? undefined;
		const targetLoadCount = targetLoadCountRef.current;
		const sendPromise = onSend({
			message,
			attachments,
			workspaceUploads,
			editingTarget: sendTarget,
		});

		const rollback = clearInputForSend(message);

		try {
			await sendPromise;
		} catch (error) {
			if (sendInFlightRef.current) {
				rollback();
			}
			sendInFlightRef.current = null;
			if (error instanceof BuiltInCommandPendingError) {
				return;
			}
			throw error;
		}
		if (!sendInFlightRef.current) {
			return;
		}
		sendInFlightRef.current = null;

		if (sendTarget?.kind === "queued") {
			removeQueuedEdit(sendTarget);
			// Saving puts the row back in the queue, and on a paused chat's
			// head it also sends it. The composer text is not a new message,
			// so the pre-edit draft is restored, unless an edit opened during
			// the save; that edit keeps the editor and the pre-edit draft.
			if (targetLoadCountRef.current === targetLoadCount) {
				restoreDraftBeforeEdit();
			}
			if (!isMobileViewport()) {
				chatInputRef.current?.focus();
			}
			return;
		}
		finalizeSuccessfulSend(sendTarget);
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
		if (target !== null) {
			return;
		}
		const sendInFlight = sendInFlightRef.current;
		if (sendInFlight && sendInFlight.editedMessageId === null) {
			const draft = restoredDraft(sendInFlight, composerContent());
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
		// The loaded target, not `target`: it lags until the load effect runs
		// and stays null for a target the editor declined.
		editingTarget: loadedTarget?.target ?? null,
		editingFileBlocks: loadedTarget?.fileBlocks ?? noFileBlocks,
		handleCancelEdit,
		handleSendFromInput,
		handleContentChange,
		handleLoadingDraftChange,
	};
}
