import { buildDisplayMessages } from "./messageHelpers";
import { assignTimelineRows } from "./timelineRows";
import type { ParsedMessageEntry } from "./types";
import {
	getTurnAttachmentsByRow,
	type TurnAttachmentsOptions,
} from "./workingBlockGrouping";

/**
 * "use memo" keys this cache on the messages and destructured flags; inlined,
 * the calls would share the timeline body's cache scope, which every stream
 * chunk invalidates.
 */
export const useTurnRows = (
	parsedMessages: readonly ParsedMessageEntry[],
	{
		hasLiveAssistant,
		hidesAttachmentRows,
		isChatActive,
		isNextTurnStarting,
		isCompacting,
		hasMoreMessages,
	}: TurnAttachmentsOptions & { hasLiveAssistant: boolean },
) => {
	"use memo";
	const rows = assignTimelineRows(
		buildDisplayMessages(parsedMessages),
		hasLiveAssistant,
	);
	return [
		rows,
		getTurnAttachmentsByRow(rows, {
			hidesAttachmentRows,
			isChatActive,
			isNextTurnStarting,
			isCompacting,
			hasMoreMessages,
		}),
	] as const;
};
