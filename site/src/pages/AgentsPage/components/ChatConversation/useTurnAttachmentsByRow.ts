import { buildDisplayMessages } from "./messageHelpers";
import { assignTimelineRows } from "./timelineRows";
import type { ParsedMessageEntry } from "./types";
import {
	getTurnAttachmentsByRow,
	type TurnAttachmentsOptions,
} from "./workingBlockGrouping";

/**
 * "use memo" keys this cache on the messages and the destructured flags, not
 * the options object. Inlined, the call would share the timeline body's
 * cache scope, which every stream chunk invalidates, so each row with a pill
 * would re-render.
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
	return getTurnAttachmentsByRow(
		assignTimelineRows(buildDisplayMessages(parsedMessages), false),
		{ hidesAttachmentRows, isChatActive, isCompacting, hasMoreMessages },
	);
};
