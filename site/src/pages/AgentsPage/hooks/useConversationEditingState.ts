import isEqual from "lodash/isEqual";
import { useEffectEvent, useLayoutEffect, useRef, useState } from "react";
import type { ChatMessagePart } from "#/api/typesGenerated";
import { isMobileViewport } from "#/utils/mobile";
import type { ChatMessageInputRef } from "../components/AgentChatInput";
import { getEditableContentPayload } from "../components/ChatConversation/messageParsing";
import type { EditingTarget } from "../components/ChatConversation/types";
import type { PendingAttachment } from "../components/ChatPageContent";
import {
	draftInputStorageKeyPrefix,
	type ParsedDraft,
	parseStoredDraft,
} from "../utils/draftStorage";
import type { ComposerMode } from "./useQueuedMessageEdit";

export class BuiltInCommandPendingError extends Error {}

/** The target whose text the editor holds, as loaded. */
type LoadedTarget = {
	target: EditingTarget;
	text: string;
	fileBlocks: readonly ChatMessagePart[];
};

const noFileBlocks: readonly ChatMessagePart[] = [];

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
	composerMode: ComposerMode;
	setComposerMode: (mode: ComposerMode) => void;
	// The row the composer edits and its content as the editor loads it.
	target: EditingTarget | null;
	targetContent: readonly ChatMessagePart[] | undefined;
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
	} = deps;
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

	const [loadedTarget, setLoadedTarget] = useState<LoadedTarget | null>(null);
	// The draft the user had before an edit opened; restored when the edit
	// ends without sending.
	const [draftBeforeEdit, setDraftBeforeEdit] = useState<ParsedDraft | null>(
		null,
	);

	const loadEditorText = (text: string, editorState: string | undefined) => {
		setDraftState({
			editorInitialValue: text,
			initialEditorState: editorState,
		});
		serializedEditorStateRef.current = editorState;
		setRemountKey((k) => k + 1);
		inputValueRef.current = text;
	};

	const persistDraft = () => {
		if (!draftStorageKey) {
			return;
		}
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
	};

	const restoreDraftBeforeEdit = () => {
		loadEditorText(draftBeforeEdit?.text ?? "", draftBeforeEdit?.editorState);
		setLoadedTarget(null);
		setDraftBeforeEdit(null);
	};

	// Loads the editor for a target change. The editor is uncontrolled, Lexical
	// behind inputValueRef, so loading it is imperative work that belongs in a
	// layout effect; it runs before paint. User actions that leave a target
	// set loadedTarget themselves, so only server-driven changes reach the
	// leave branch: dirty text stays as a new-message draft; clean text gives
	// the draft back and the composer counts as untouched again, so it follows
	// the row the server marks now.
	const loadTargetIntoEditor = useEffectEvent(() => {
		if (isEqual(target, loadedTarget?.target ?? null)) {
			return;
		}
		const textModified =
			loadedTarget !== null && inputValueRef.current !== loadedTarget.text;
		if (target === null) {
			if (textModified) {
				setLoadedTarget(null);
				setDraftBeforeEdit(null);
				persistDraft();
				setComposerMode("draft");
			} else {
				restoreDraftBeforeEdit();
				setComposerMode(undefined);
			}
			return;
		}
		const { text, fileBlocks } = getEditableContentPayload(targetContent);
		if (loadedTarget === null) {
			// localStorage holds the serialized state handleContentChange
			// persisted; the initialEditorState React state is stale.
			setDraftBeforeEdit({
				text: inputValueRef.current,
				editorState: draftStorageKey
					? parseStoredDraft(localStorage.getItem(draftStorageKey)).editorState
					: undefined,
			});
		} else if (textModified) {
			setDraftBeforeEdit({
				text: inputValueRef.current,
				editorState: serializedEditorStateRef.current,
			});
		}
		loadEditorText(text, undefined);
		setLoadedTarget({ target, text, fileBlocks: fileBlocks ?? noFileBlocks });
	});
	useLayoutEffect(() => {
		loadTargetIntoEditor();
	}, [target]);

	const handleCancelEdit = () => {
		restoreDraftBeforeEdit();
		setComposerMode("draft");
	};

	// Clears the composer for an in-flight edit and returns a rollback
	// function that restores the editing draft if the send fails.
	const clearInputForEdit = (message: string) => {
		const snapshot = {
			editorState: serializedEditorStateRef.current,
			loadedTarget,
		};

		inputValueRef.current = "";
		chatInputRef.current?.clear();
		setLoadedTarget(null);
		setComposerMode("draft");

		return () => {
			loadEditorText(message, snapshot.editorState);
			setLoadedTarget(snapshot.loadedTarget);
			if (snapshot.loadedTarget) {
				setComposerMode(snapshot.loadedTarget.target);
			}
		};
	};

	// Clears all input and editing state after a successful send.
	const finalizeSuccessfulSend = (sentTarget: EditingTarget | undefined) => {
		inputValueRef.current = "";
		chatInputRef.current?.clear();
		if (!isMobileViewport()) {
			chatInputRef.current?.focus();
		}
		serializedEditorStateRef.current = undefined;
		if (draftStorageKey) {
			localStorage.removeItem(draftStorageKey);
		}
		if (sentTarget !== undefined) {
			setDraftBeforeEdit(null);
		}
	};

	// Wraps the parent onSend to clear local input/editing state.
	const handleSendFromInput = async (
		message: string,
		attachments?: readonly PendingAttachment[],
	) => {
		const sendTarget = target ?? undefined;
		const sendPromise = onSend(message, attachments, sendTarget);

		// For edits, clear input immediately and prepare a rollback in
		// case the send fails.
		const rollback =
			sendTarget !== undefined ? clearInputForEdit(message) : undefined;

		try {
			await sendPromise;
		} catch (error) {
			if (error instanceof BuiltInCommandPendingError) {
				return;
			}
			rollback?.();
			throw error;
		}

		if (sendTarget?.kind === "queued") {
			// A saved queued row does not start a turn; the pre-edit draft
			// is restored.
			restoreDraftBeforeEdit();
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
		// The editor seed echoes the text the ref already holds; anything
		// else is user input.
		const isUserInput = content !== inputValueRef.current;
		inputValueRef.current = content;
		serializedEditorStateRef.current = serializedEditorState;
		if (isUserInput && composerMode === undefined) {
			setComposerMode(target ?? "draft");
		}

		// Don't overwrite the persisted draft while editing a message.
		// The original draft is saved in React state and should survive a cancel.
		if (target !== null) {
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
		editingTarget: target,
		editingFileBlocks: loadedTarget?.fileBlocks ?? noFileBlocks,
		handleCancelEdit,
		handleSendFromInput,
		handleContentChange,
		handleLoadingDraftChange,
	};
}
