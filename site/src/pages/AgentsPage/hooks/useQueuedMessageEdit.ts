import { useState } from "react";
import { getErrorStatus } from "#/api/errors";
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
 * The queued row rendered as under edit. A pending marker request applies
 * before it settles, so the row reacts to Edit and Cancel edit without
 * waiting for the server.
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
 * Edit opens the row in the composer before the begin request settles; a
 * refused begin restores the composer mode from before Edit.
 */
export function useQueuedMessageEdit(deps: {
	store: ChatStore;
	composerMode: ComposerMode;
	setComposerMode: React.Dispatch<React.SetStateAction<ComposerMode>>;
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
	// The choice of the latest Edit until the begin's handlers below have
	// applied its outcome. While the composer holds that choice it stays the
	// target, so the editor never sees it go away before a refusal restores
	// the earlier mode; react-query can report the settled request first.
	const [unappliedEdit, setUnappliedEdit] = useState<EditingTarget | null>(
		null,
	);
	const composerTarget =
		composerMode === unappliedEdit
			? unappliedEdit
			: deriveComposerTarget(composerMode, serverMarkedID, marker, isOwner);

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

		const editMode: EditingTarget = { kind: "queued", id };
		setComposerMode(editMode);
		setUnappliedEdit(editMode);

		const applied = () =>
			setUnappliedEdit((current) => (current === editMode ? null : current));
		void deps
			.setQueuedMessageEditing(id, true)
			.then(applied, (error: unknown) => {
				// A refused begin returns the composer to the mode before Edit, so
				// it does not open the row later when another client edits it,
				// unless a later choice replaced this one. A 404 dropped the row,
				// which closes the edit on its own.
				if (getErrorStatus(error) !== 404) {
					setComposerMode((current) =>
						current === editMode ? composerMode : current,
					);
				}
				applied();
			});
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
