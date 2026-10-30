import isEqual from "lodash/isEqual";
import { useEffectEvent, useLayoutEffect, useRef, useState } from "react";
import type { ChatMessagePart } from "#/api/typesGenerated";
import { isMobileViewport } from "#/utils/mobile";
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

/** The target whose text the editor holds, as loaded. */
type LoadedTarget = {
	target: EditingTarget;
	text: string;
	fileBlocks: readonly ChatMessagePart[];
};

const noFileBlocks: readonly ChatMessagePart[] = [];

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
	// Holds the editor state of an open queued edit, so typed text survives
	// a reload or a return to the chat until the edit ends.
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

	// The content last loaded into the editor: the stored draft, a target's
	// text, a restored draft, or the empty editor after a send. The composer
	// counts as changed while the editor differs from it.
	const loadedContentRef = useRef<ParsedDraft>({
		text: editorInitialValue,
		editorState: initialEditorState,
	});
	const hasFileReferencesRef = useRef(false);

	const loadEditorText = (text: string, editorState: string | undefined) => {
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

	const restoreDraftBeforeEdit = () => {
		removeQueuedEdit(loadedTarget?.target);
		loadEditorText(draftBeforeEdit?.text ?? "", draftBeforeEdit?.editorState);
		setLoadedTarget(null);
		setDraftBeforeEdit(null);
	};

	// Loads the target into the editor when it differs from the loaded
	// target. A layout effect because the Lexical editor is uncontrolled and
	// the load must land before paint. Whether the editor changed is checked
	// only when the target changes, so text typed and deleted again does not
	// count. A changed editor keeps its content: if its target goes away it
	// stays as a new-message draft, and in follow mode it does not load a
	// newly marked row. If an unchanged edit's target goes away, the pre-edit
	// draft comes back and the composer follows the row the server marks,
	// unless the composer mode was already set to draft, as a refused Edit
	// does.
	// The pre-edit draft is captured once per session. A queued row with
	// text saved from an earlier mount loads that text, which counts as
	// changed against the row's content.
	const loadTargetIntoEditor = useEffectEvent(() => {
		if (isEqual(target, loadedTarget?.target ?? null)) {
			return;
		}

		removeQueuedEdit(loadedTarget?.target);

		if (isEditorChanged() && (target === null || composerMode === "follow")) {
			if (loadedTarget !== null) {
				onLoadedTargetChange?.();
				setLoadedTarget(null);
				setDraftBeforeEdit(null);
				persistDraft();
			}
			setComposerMode("draft");
			return;
		}

		onLoadedTargetChange?.();
		if (target === null) {
			restoreDraftBeforeEdit();
			if (composerMode !== "draft") {
				setComposerMode("follow");
			}
			return;
		}

		const { text, fileBlocks, editorState } =
			getEditableContentPayload(targetContent);
		if (draftBeforeEdit === null) {
			// localStorage holds the serialized state handleContentChange
			// persisted; the initialEditorState React state is stale.
			setDraftBeforeEdit({
				text: inputValueRef.current,
				editorState: parseStoredDraft(localStorage.getItem(draftStorageKey))
					.editorState,
			});
		}
		loadEditorText(text, editorState);
		const savedEdit =
			target.kind === "queued"
				? localStorage.getItem(queuedEditStorageKey(target.id))
				: null;
		if (savedEdit !== null) {
			const saved = parseStoredDraft(savedEdit);
			loadEditorText(saved.text, saved.editorState);
			loadedContentRef.current = { text, editorState };
		}

		setLoadedTarget({ target, text, fileBlocks: fileBlocks ?? noFileBlocks });
		targetLoadCountRef.current++;
	});
	useLayoutEffect(() => {
		loadTargetIntoEditor();
	}, [target, loadedTarget]);

	// Ends the edit and gives the draft from before it back. next is the
	// composer mode afterwards; a target there opens that edit.
	const handleCancelEdit = (next: ComposerMode = "draft") => {
		restoreDraftBeforeEdit();
		setComposerMode(next);
	};

	// Clears the composer for an in-flight edit and returns a rollback
	// function that restores the editing draft if the send fails.
	const clearInputForEdit = (message: string) => {
		const snapshot = {
			editorState: serializedEditorStateRef.current,
			loadedTarget,
			loadedContent: loadedContentRef.current,
			hasFileReferences: hasFileReferencesRef.current,
		};

		inputValueRef.current = "";
		chatInputRef.current?.clear();
		setLoadedTarget(null);
		setComposerMode("draft");

		return () => {
			loadEditorText(message, snapshot.editorState);
			loadedContentRef.current = snapshot.loadedContent;
			hasFileReferencesRef.current = snapshot.hasFileReferences;
			setLoadedTarget(snapshot.loadedTarget);
			if (snapshot.loadedTarget) {
				setComposerMode(snapshot.loadedTarget.target);
			}
		};
	};

	const finalizeSuccessfulSend = (sentTarget: EditingTarget | undefined) => {
		inputValueRef.current = "";
		chatInputRef.current?.clear();
		if (!isMobileViewport()) {
			chatInputRef.current?.focus();
		}
		serializedEditorStateRef.current = undefined;
		loadedContentRef.current = { text: "", editorState: undefined };
		localStorage.removeItem(draftStorageKey);
		if (sentTarget !== undefined) {
			setDraftBeforeEdit(null);
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

		const loaded = loadedTarget?.target;
		if (loaded?.kind === "queued") {
			if (isEditorChanged() && (content.trim() || hasFileReferences)) {
				try {
					localStorage.setItem(
						queuedEditStorageKey(loaded.id),
						serializedEditorState,
					);
				} catch {
					// QuotaExceededError, silently discard the saved edit.
				}
			} else {
				removeQueuedEdit(loaded);
			}
		}

		// Don't overwrite the persisted draft while editing a message.
		// The original draft is saved in React state and should survive a cancel.
		if (target !== null) {
			return;
		}

		writeDraft(content, serializedEditorState, hasFileReferences);
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
