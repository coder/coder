import { useRef, useState } from "react";
import type * as TypesGen from "#/api/typesGenerated";
import { reconcileQueuedEditMarker } from "../components/ChatConversation/chatQueueReconciliation";
import {
	type ChatStore,
	useChatSelector,
} from "../components/ChatConversation/chatStore";
import type { LocalQueuedEditMarker } from "../components/ChatConversation/types";
import type { useConversationEditingState } from "./useConversationEditingState";

/**
 * Owns the queued-edit orchestration: which queued row the composer edits,
 * the marker this client expects ahead of the server, and how the composer
 * reacts when the server marker disagrees. The page owns the requests and
 * passes them in. The render-time block adjusts state during render on
 * purpose so the composer follows the store snapshot in the same render;
 * a StrictMode double render is idempotent.
 */
export function useQueuedMessageEdit(deps: {
	store: ChatStore;
	composer: Pick<
		ReturnType<typeof useConversationEditingState>,
		| "editingTarget"
		| "handleBeginEdit"
		| "handleCancelEdit"
		| "leaveEditKeepingText"
	>;
	// The page resets its edit-only picker state before beginning.
	beginEditFromRow: (row: TypesGen.ChatQueuedMessage) => void;
	patchQueuedMessage: (
		id: number,
		req: TypesGen.EditChatQueuedMessageRequest,
		failureMessage: string,
	) => Promise<void>;
	restore: {
		queuedMessages: readonly TypesGen.ChatQueuedMessage[] | undefined;
		ready: boolean;
		allowed: boolean;
	};
}): {
	queuedEditTargetID: number | null;
	localQueuedEditMarker: LocalQueuedEditMarker | undefined;
	handleEditQueuedMessage: (id: number) => void;
	handleEndQueuedMessageEdit: (id: number) => Promise<void>;
	handleCancelEdit: () => void;
} {
	const { store, composer, beginEditFromRow, patchQueuedMessage, restore } =
		deps;
	const [queuedEditRestored, setQueuedEditRestored] = useState(false);
	const [queuedEditMarkerSeenID, setQueuedEditMarkerSeenID] = useState<
		number | null
	>(null);
	const [failedQueuedEditBeginID, setFailedQueuedEditBeginID] = useState<
		number | null
	>(null);
	const [endingQueuedEditID, setEndingQueuedEditID] = useState<number | null>(
		null,
	);
	const queuedEditBeginAttemptRef = useRef(0);

	const queuedEditTargetID =
		composer.editingTarget?.kind === "queued"
			? composer.editingTarget.id
			: null;
	const queuedEditTargetRow = useChatSelector(store, (s) =>
		s.queuedMessages.find((row) => row.id === queuedEditTargetID),
	);
	const rowUnderEditOnServer = useChatSelector(store, (s) =>
		s.queuedMessages.find((row) => row.editing_since),
	);
	const endingRowStillMarked = useChatSelector(store, (s) =>
		s.queuedMessages.some(
			(row) => row.id === endingQueuedEditID && Boolean(row.editing_since),
		),
	);

	// Composer follows the server marker: restore once from the first fetched
	// queue; a marker cleared after it was seen keeps the typed text; a row
	// gone before its marker was seen, or a failed begin, restores the draft
	// or follows the row the server marks.
	if (!queuedEditRestored && restore.ready && restore.queuedMessages) {
		setQueuedEditRestored(true);
		const markedRow = restore.queuedMessages.find((row) => row.editing_since);
		if (restore.allowed && markedRow && composer.editingTarget === null) {
			beginEditFromRow(markedRow);
		}
	}
	const reconciled = reconcileQueuedEditMarker(
		queuedEditTargetID,
		queuedEditTargetRow,
		queuedEditMarkerSeenID,
	);
	if (reconciled.markerSeenID !== queuedEditMarkerSeenID) {
		setQueuedEditMarkerSeenID(reconciled.markerSeenID);
	}
	const beginFailed =
		failedQueuedEditBeginID !== null &&
		failedQueuedEditBeginID === queuedEditTargetID;
	if (failedQueuedEditBeginID !== null) {
		setFailedQueuedEditBeginID(null);
	}
	if (reconciled.lost === "marker_cleared") {
		composer.leaveEditKeepingText();
	} else if (reconciled.lost === "row_gone_before_marker" || beginFailed) {
		if (rowUnderEditOnServer) {
			beginEditFromRow(rowUnderEditOnServer);
		} else {
			composer.handleCancelEdit();
		}
	}
	if (endingQueuedEditID !== null && !endingRowStillMarked) {
		setEndingQueuedEditID(null);
	}

	let localQueuedEditMarker: LocalQueuedEditMarker | undefined;
	if (queuedEditTargetID !== null) {
		localQueuedEditMarker = { id: queuedEditTargetID, editing: true };
	} else if (endingQueuedEditID !== null) {
		localQueuedEditMarker = { id: endingQueuedEditID, editing: false };
	}

	const requestEndQueuedEdit = async (id: number) => {
		setEndingQueuedEditID(id);
		try {
			await patchQueuedMessage(
				id,
				{ editing: false },
				"Failed to cancel the edit.",
			);
		} catch (error) {
			setEndingQueuedEditID((current) => (current === id ? null : current));
			throw error;
		}
	};

	const handleEditQueuedMessage = (id: number) => {
		const row = store
			.getSnapshot()
			.queuedMessages.find((message) => message.id === id);
		if (!row || queuedEditTargetID === id) {
			return;
		}
		beginEditFromRow(row);
		// The same row can be re-begun before a prior begin settles; only the
		// latest attempt's failure may act.
		const attempt = ++queuedEditBeginAttemptRef.current;
		patchQueuedMessage(
			id,
			{ editing: true },
			"Failed to start editing the queued message.",
		).catch(() => {
			if (attempt === queuedEditBeginAttemptRef.current) {
				setFailedQueuedEditBeginID(id);
			}
		});
	};

	const handleEndQueuedMessageEdit = (id: number) => {
		if (id === queuedEditTargetID) {
			composer.handleCancelEdit();
		}
		return requestEndQueuedEdit(id);
	};

	const handleCancelEdit = () => {
		if (queuedEditTargetID !== null) {
			void handleEndQueuedMessageEdit(queuedEditTargetID).catch(
				() => undefined,
			);
			return;
		}
		if (
			composer.editingTarget?.kind === "history" &&
			rowUnderEditOnServer &&
			restore.allowed
		) {
			beginEditFromRow(rowUnderEditOnServer);
			return;
		}
		composer.handleCancelEdit();
	};

	return {
		queuedEditTargetID,
		localQueuedEditMarker,
		handleEditQueuedMessage,
		handleEndQueuedMessageEdit,
		handleCancelEdit,
	};
}
