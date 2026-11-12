import { useState } from "react";
import {
	type ChatStore,
	selectQueuedMessages,
	useChatSelector,
} from "../components/ChatConversation/chatStore";
import type { EditingTarget } from "../components/ChatConversation/types";

/**
 * What the user chose for the composer in this chat session. "follow" edits
 * the marked row, "draft" writes a new message, and a target edits that row.
 * The marked row is the one the shown queue marks under edit, which this
 * tab's pending begin or end changes at the click.
 */
export type ComposerMode = EditingTarget | "draft" | "follow";

/**
 * The row the composer edits. In follow mode it is the owner's marked row. A
 * queued choice holds while that row is marked, and is null otherwise.
 * useConversationEditingState decides what the editor does with the target.
 */
export const deriveComposerTarget = (
	composerMode: ComposerMode,
	markedID: number | null,
	isOwner: boolean,
): EditingTarget | null => {
	if (composerMode === "follow") {
		return isOwner && markedID !== null
			? { kind: "queued", id: markedID }
			: null;
	}
	if (composerMode === "draft") {
		return null;
	}
	if (composerMode.kind === "history") {
		return composerMode;
	}
	return markedID === composerMode.id ? composerMode : null;
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
	setQueuedMessageEditing: (id: number, editing: boolean) => Promise<void>;
}): {
	// The row that the shown queue marks as under edit.
	markedID: number | null;
	composerTarget: EditingTarget | null;
	handleEditQueuedMessage: (id: number) => void;
	handleEndQueuedMessageEdit: (id: number) => Promise<void>;
} {
	const { store, composerMode, setComposerMode, isOwner } = deps;
	const markedID = useChatSelector(
		store,
		(s) => selectQueuedMessages(s).find((row) => row.editing_since)?.id ?? null,
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
			: deriveComposerTarget(composerMode, markedID, isOwner);

	const handleEditQueuedMessage = (id: number) => {
		const row = selectQueuedMessages(store.getSnapshot()).find(
			(message) => message.id === id,
		);
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
		void deps.setQueuedMessageEditing(id, true).then(applied, () => {
			// A refused begin returns the composer to the mode before Edit, so
			// it does not open the row later when another client edits it,
			// unless a later choice replaced this one.
			setComposerMode((current) =>
				current === editMode ? composerMode : current,
			);
			applied();
		});
	};

	const handleEndQueuedMessageEdit = (id: number) =>
		deps.setQueuedMessageEditing(id, false);

	return {
		markedID,
		composerTarget,
		handleEditQueuedMessage,
		handleEndQueuedMessageEdit,
	};
}
