import { buildDisplayMessages } from "./messageHelpers";
import { assignTimelineRows } from "./timelineRows";
import type { ParsedMessageEntry } from "./types";
import {
	getTurnAttachmentsByRow,
	type TurnAttachmentsOptions,
} from "./workingBlockGrouping";

/**
 * "use memo" keys this cache on the messages and the destructured flags, not
 * the options object. Inlined in the timeline body, the call gets no cache,
 * so every stream chunk would re-render each row with a pill.
 */
export const useTurnAttachmentsByRow = (
	parsedMessages: readonly ParsedMessageEntry[],
	{
		hidesAttachmentRows,
		isChatActive,
		isCompacting,
		hasMoreMessages,
	}: TurnAttachmentsOptions,
) => {
	"use memo";
	const withLiveRow = false;
	return getTurnAttachmentsByRow(
		assignTimelineRows(buildDisplayMessages(parsedMessages), withLiveRow),
		{ hidesAttachmentRows, isChatActive, isCompacting, hasMoreMessages },
	);
};
