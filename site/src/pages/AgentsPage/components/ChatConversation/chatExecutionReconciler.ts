import type { ChatExecutionSnapshotEvent } from "#/api/queries/chats";
import type * as TypesGen from "#/api/typesGenerated";
import { type ChatPreviewPart, toChatPreviewPart } from "./chatExecutionStream";

export type ChatExecutionReconcilerPorts = {
	applyPreviewPart: (part: ChatPreviewPart) => void;
	beginHistoryReplacement: () => void;
	resetPreview: () => void;
	commitMessage: (message: TypesGen.ChatMessage) => void;
	replaceQueue: (messages: readonly TypesGen.ChatQueuedMessage[]) => void;
	applyActionRequired: (state: TypesGen.ChatStreamActionRequired) => void;
	applyStatus: (status: TypesGen.ChatStatus) => void;
	applyError: (error: TypesGen.ChatError | undefined) => void;
	applyRetry: (retry: TypesGen.ChatStreamRetry) => void;
};

export const reconcileChatExecutionSnapshotEvent = ({
	event,
	connectionEpoch,
	ports,
}: {
	event: ChatExecutionSnapshotEvent;
	connectionEpoch: number;
	ports: ChatExecutionReconcilerPorts;
}): void => {
	switch (event.type) {
		case "message_part": {
			const part = toChatPreviewPart(event, connectionEpoch);
			if (part) {
				ports.applyPreviewPart(part);
			}
			return;
		}
		case "history_reset":
			ports.beginHistoryReplacement();
			return;
		case "preview_reset":
			ports.resetPreview();
			return;
		case "message":
			if (event.message) {
				ports.commitMessage(event.message);
			}
			return;
		case "queue_update":
			ports.replaceQueue(event.queued_messages ?? []);
			return;
		case "action_required":
			if (event.action_required) {
				ports.applyActionRequired(event.action_required);
			}
			return;
		case "status":
			if (event.status) {
				ports.applyStatus(event.status.status);
			}
			return;
		case "error":
			ports.applyError(event.error);
			return;
		case "retry":
			if (event.retry) {
				ports.applyRetry(event.retry);
			}
			return;
	}
};
