import type { EditChatQueuedMessageVariables } from "#/api/queries/chats";
import {
	type ChatStore,
	useChatSelector,
} from "../components/ChatConversation/chatStore";
import type { EditingTarget } from "../components/ChatConversation/types";

/**
 * What the user chose for the composer in this chat session. "follow" edits
 * the row the server marks, "draft" writes a new message, and a target edits
 * that row.
 */
export type ComposerMode = EditingTarget | "draft" | "follow";

/** The latest marker request's state, as reported by its useMutation. */
export type MarkerRequest = {
	isPending: boolean;
	variables: EditChatQueuedMessageVariables | undefined;
};

const beginRequestedOn = (marker: MarkerRequest, id: number): boolean =>
	marker.variables?.queuedMessageId === id &&
	marker.variables.req.editing === true;

/**
 * The queued row rendered as under edit. A pending begin shows its row at
 * once and a pending end on the marked row hides it at once; otherwise the
 * store marker decides.
 */
export const deriveQueuedMessageUnderEditID = (
	serverMarkedID: number | null,
	marker: MarkerRequest,
): number | null => {
	if (marker.isPending && marker.variables) {
		const { queuedMessageId, req } = marker.variables;
		if (req.editing === true) {
			return queuedMessageId;
		}
		if (req.editing === false && queuedMessageId === serverMarkedID) {
			return null;
		}
	}
	return serverMarkedID;
};

/**
 * The row the composer edits. In follow mode it is the owner's marked row. A
 * queued choice holds while the server marks that row or its begin is in
 * flight, and is null otherwise. useConversationEditingState decides what
 * the editor does with the target.
 */
export const deriveComposerTarget = (
	composerMode: ComposerMode,
	serverMarkedID: number | null,
	marker: MarkerRequest,
	isOwner: boolean,
): EditingTarget | null => {
	if (composerMode === "follow") {
		return isOwner && serverMarkedID !== null
			? { kind: "queued", id: serverMarkedID }
			: null;
	}
	if (composerMode === "draft") {
		return null;
	}
	if (composerMode.kind === "history") {
		return composerMode;
	}
	const beginPending =
		marker.isPending && beginRequestedOn(marker, composerMode.id);
	return serverMarkedID === composerMode.id || beginPending
		? composerMode
		: null;
};

/**
 * Derives the queued-edit view from the store marker, the user's composer
 * choice and the latest marker request.
 */
export function useQueuedMessageEdit(deps: {
	store: ChatStore;
	composerMode: ComposerMode;
	setComposerMode: (mode: ComposerMode) => void;
	isOwner: boolean;
	marker: MarkerRequest;
	setQueuedMessageEditing: (id: number, editing: boolean) => Promise<void>;
}): {
	serverMarkedID: number | null;
	queuedMessageUnderEditID: number | null;
	composerTarget: EditingTarget | null;
	handleEditQueuedMessage: (id: number) => void;
	handleEndQueuedMessageEdit: (id: number) => Promise<void>;
} {
	const { store, composerMode, setComposerMode, isOwner, marker } = deps;
	const serverMarkedID = useChatSelector(
		store,
		(s) => s.queuedMessages.find((row) => row.editing_since)?.id ?? null,
	);
	const composerTarget = deriveComposerTarget(
		composerMode,
		serverMarkedID,
		marker,
		isOwner,
	);

	const handleEditQueuedMessage = (id: number) => {
		const row = store
			.getSnapshot()
			.queuedMessages.find((message) => message.id === id);
		if (
			!row ||
			(composerTarget?.kind === "queued" && composerTarget.id === id)
		) {
			return;
		}
		setComposerMode({ kind: "queued", id });
		void deps.setQueuedMessageEditing(id, true).catch(() => undefined);
	};

	const handleEndQueuedMessageEdit = (id: number) =>
		deps.setQueuedMessageEditing(id, false);

	return {
		serverMarkedID,
		queuedMessageUnderEditID: deriveQueuedMessageUnderEditID(
			serverMarkedID,
			marker,
		),
		composerTarget,
		handleEditQueuedMessage,
		handleEndQueuedMessageEdit,
	};
}
