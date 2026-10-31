import { useLayoutEffect, useRef, useState } from "react";
import type { ChatMessagePart } from "#/api/typesGenerated";
import { isMobileViewport } from "#/utils/mobile";
import type { ChatMessageInputRef } from "../components/AgentChatInput";
import type { EditingTarget } from "../components/ChatConversation/types";
import {
	getAsyncAnswerPrefix,
	parseAsyncAnswer,
} from "../components/ChatElements/tools/asyncAnswer";
import type { SendChatMessageOptions } from "../components/ChatPageContent";
import type { ChatComposerContentPart } from "../utils/chatInputContent";
import {
	draftInputStorageKeyPrefix,
	type ParsedDraft,
	parseStoredDraft,
} from "../utils/draftStorage";

export class BuiltInCommandPendingError extends Error {}

export type SendChatTurnOptions = SendChatMessageOptions & {
	editingTarget?: EditingTarget;
	/** Content to send in place of the composer's current parts. */
	composerParts?: readonly ChatComposerContentPart[];
};

// Returns undefined for an empty composer, so the send falls back to the
// message text, which already starts with the prefix line.
const prependAnswerPrefix = (
	prefix: string,
	parts: readonly ChatComposerContentPart[],
): readonly ChatComposerContentPart[] | undefined => {
	if (parts.length === 0) {
		return undefined;
	}
	const [firstPart, ...restParts] = parts;
	if (firstPart.type === "text") {
		return [{ type: "text", text: prefix + firstPart.text }, ...restParts];
	}
	return [{ type: "text", text: prefix }, ...parts];
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
	// The composer shows an async answer without its prefix line, so the
	// send adds the line back to keep the answer tied to its question.
	const [editingAnswerPrefix, setEditingAnswerPrefix] = useState("");
	// Counts opened edits, so an edit that finishes sending after the user
	// opened another one leaves the newer edit's composer and state alone.
	const openedEditCountRef = useRef(0);

	const handleBeginEdit = (
		target: EditingTarget,
		text: string,
		fileBlocks?: readonly ChatMessagePart[],
		editorState?: string,
	) => {
		openedEditCountRef.current += 1;
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
		const asyncAnswer = parseAsyncAnswer(text);
		const editText = asyncAnswer?.body ?? text;
		setEditingAnswerPrefix(
			asyncAnswer ? getAsyncAnswerPrefix(asyncAnswer.toolCallId) : "",
		);
		setEditingTarget(target);
		setDraftState({
			editorInitialValue: editText,
			initialEditorState: editorState,
		});
		serializedEditorStateRef.current = editorState;
		setRemountKey((k) => k + 1);
		inputValueRef.current = editText;
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
		setEditingAnswerPrefix("");
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
		localStorage.removeItem(draftStorageKey);
		if (target !== undefined) {
			setDraftBeforeEdit(null);
			setEditingFileBlocks([]);
			setEditingAnswerPrefix("");
		}
	};

	// Wraps the parent onSend to clear local input/editing state.
	const handleSendFromInput = async ({
		message,
		attachments,
		workspaceUploads,
	}: SendChatMessageOptions) => {
		const target = editingTarget ?? undefined;
		const answerPrefix = target !== undefined ? editingAnswerPrefix : "";
		const openedEditCount = openedEditCountRef.current;
		const sendPromise = onSend({
			message: answerPrefix + message,
			attachments,
			workspaceUploads,
			editingTarget: target,
			composerParts: answerPrefix
				? prependAnswerPrefix(
						answerPrefix,
						chatInputRef.current?.getContentParts() ?? [],
					)
				: undefined,
		});

		const rollback =
			target !== undefined ? clearInputForEdit(message) : undefined;

		try {
			await sendPromise;
		} catch (error) {
			if (error instanceof BuiltInCommandPendingError) {
				return;
			}
			if (openedEditCountRef.current === openedEditCount) {
				rollback?.();
			}
			throw error;
		}

		if (
			target !== undefined &&
			openedEditCountRef.current !== openedEditCount
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

		// Don't overwrite the persisted draft while editing a message.
		// The original draft is saved in React state and should survive a cancel.
		if (editingTarget !== null) {
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
