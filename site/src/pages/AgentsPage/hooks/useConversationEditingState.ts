import isEqual from "lodash/isEqual";
import {
	useEffect,
	useEffectEvent,
	useLayoutEffect,
	useRef,
	useState,
} from "react";
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

/** The target whose text the editor holds, as loaded. */
type LoadedTarget = {
	target: EditingTarget;
	text: string;
	fileBlocks: readonly ChatMessagePart[];
	// The target's content as loaded, in the form SavedQueuedEdit.base uses.
	base: string;
	// The saved edit text loaded with the target, if any.
	savedEdit?: ParsedDraft;
};

const noFileBlocks: readonly ChatMessagePart[] = [];

/** Text typed in a queued edit, saved until the edit ends. */
type SavedQueuedEdit = {
	// The row content it was typed against.
	base: string;
	state: string;
};

const readSavedQueuedEdit = (raw: string | null): SavedQueuedEdit | null => {
	try {
		const saved = JSON.parse(raw ?? "null");
		if (typeof saved?.base === "string" && typeof saved.state === "string") {
			return saved;
		}
	} catch {
		// Not a saved edit.
	}
	return null;
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
	target: EditingTarget | null;
	loadedTarget: LoadedTarget | null;
	loadedContent: ParsedDraft;
	hasFileReferences: boolean;
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

	const [loadedTarget, setLoadedTarget] = useState<LoadedTarget | null>(null);
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

	// Stores draft, or removes the stored draft when draft is empty, and
	// records the stored value in storedDraftRef for the check after unmount.
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

	const writeDraft = (
		text: string,
		serializedEditorState: string,
		hasFileReferences: boolean,
	) =>
		saveDraft(
			text.trim() || hasFileReferences ? serializedEditorState : undefined,
		);

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
	// count. A changed editor, like one with a request in flight, keeps its
	// content: if its target goes away it stays as a new-message draft, and in
	// follow mode it does not load a newly marked row. If an unchanged edit's
	// target goes away, the pre-edit draft comes back and the composer follows
	// the row the server marks, unless the composer mode was already set to
	// draft, as a refused Edit does.
	// The pre-edit draft is captured once per session. A queued row with
	// saved text typed against its current content loads that text, which
	// counts as changed against the row's content. When that edit ends, the
	// loaded saved text counts as unchanged, so only text typed since the
	// load stays as a draft.
	const loadTargetIntoEditor = useEffectEvent(() => {
		if (isEqual(target, loadedTarget?.target ?? null)) {
			return;
		}

		if (loadedTarget?.savedEdit !== undefined) {
			loadedContentRef.current = loadedTarget.savedEdit;
		}
		removeQueuedEdit(loadedTarget?.target);

		if (
			(isEditorChanged() || requestInFlightRef.current !== null) &&
			(target === null || composerMode === "follow")
		) {
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
		const base = JSON.stringify(targetContent ?? []);
		const savedEdit =
			target.kind === "queued"
				? readSavedQueuedEdit(
						localStorage.getItem(queuedEditStorageKey(target.id)),
					)
				: null;
		let saved: ParsedDraft | undefined;
		if (savedEdit?.base === base) {
			saved = parseStoredDraft(savedEdit.state);
			loadEditorText(saved.text, saved.editorState);
			loadedContentRef.current = { text, editorState };
		} else {
			removeQueuedEdit(target);
		}

		setLoadedTarget({
			target,
			text,
			fileBlocks: fileBlocks ?? noFileBlocks,
			base,
			savedEdit: saved,
		});
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

	// Records the request in flight and returns a rollback for its failure. A
	// send clears the composer and an edit shows the compose draft, so the
	// user can write a new message during the request. After a failed edit,
	// what the composer showed during the request becomes the compose draft
	// that Cancel restores.
	const removeSubmittedContent = (
		message: string,
		requestTarget: EditingTarget | undefined,
	) => {
		const snapshot: RequestInFlight = {
			message,
			editorState: serializedEditorStateRef.current,
			target: requestTarget ?? null,
			loadedTarget,
			loadedContent: loadedContentRef.current,
			hasFileReferences: hasFileReferencesRef.current,
		};

		requestInFlightRef.current = snapshot;
		if (snapshot.target === null) {
			chatInputRef.current?.clear();
			inputValueRef.current = "";
			hasFileReferencesRef.current = false;
			loadedContentRef.current = { text: "", editorState: undefined };
		} else {
			loadEditorText(draftBeforeEdit?.text ?? "", draftBeforeEdit?.editorState);
			setLoadedTarget(null);
			setComposerMode("draft");
		}

		return () => {
			const shown = composerContent();
			if (snapshot.target === null) {
				const restored = restoredDraft(snapshot, shown);
				loadEditorText(restored.text, restored.editorState);
				hasFileReferencesRef.current =
					snapshot.hasFileReferences || shown.hasFileReferences;
			} else {
				setDraftBeforeEdit({
					text: shown.text,
					editorState: shown.editorState,
				});
				loadEditorText(snapshot.message, snapshot.editorState);
				hasFileReferencesRef.current = snapshot.hasFileReferences;
				setLoadedTarget(snapshot.loadedTarget);
				if (snapshot.loadedTarget) {
					setComposerMode(snapshot.loadedTarget.target);
				}
			}
			loadedContentRef.current = snapshot.loadedContent;
		};
	};

	const handleSendFromInput = async ({
		message,
		attachments,
		workspaceUploads,
	}: SendChatMessageOptions) => {
		const sendTarget = target ?? undefined;
		const sendPromise = onSend({
			message,
			attachments,
			workspaceUploads,
			editingTarget: sendTarget,
		});

		const rollback = removeSubmittedContent(message, sendTarget);

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

		// The composer holds the chat's draft: for a send what was typed since,
		// for an edit the compose draft as edited since.
		persistDraft();
		if (sendTarget !== undefined) {
			removeQueuedEdit(sendTarget);
			setDraftBeforeEdit(null);
		}
	};

	const handleContentChange = (
		content: string,
		serializedEditorState: string,
		hasFileReferences: boolean,
	) => {
		inputValueRef.current = content;
		serializedEditorStateRef.current = serializedEditorState;
		hasFileReferencesRef.current = hasFileReferences;

		if (loadedTarget?.target.kind === "queued") {
			const loaded = loadedTarget.target;
			if (isEditorChanged() && (content.trim() || hasFileReferences)) {
				const savedEdit: SavedQueuedEdit = {
					base: loadedTarget.base,
					state: serializedEditorState,
				};
				try {
					localStorage.setItem(
						queuedEditStorageKey(loaded.id),
						JSON.stringify(savedEdit),
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
		const requestInFlight = requestInFlightRef.current;
		if (requestInFlight && requestInFlight.target === null) {
			const draft = restoredDraft(requestInFlight, composerContent());
			saveDraft(draft.editorState ?? draft.text);
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
		// An edit opened during a request would be overwritten by the
		// request's rollback, so callers do not open one then.
		isRequestInFlight: () => requestInFlightRef.current !== null,
	};
}
