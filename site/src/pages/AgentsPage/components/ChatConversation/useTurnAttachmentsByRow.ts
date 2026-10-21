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
 * cache, which every stream chunk resets, re-rendering each row with a pill.
 */
export const useTurnAttachmentsByRow = (
	parsedMessages: readonly ParsedMessageEntry[],
	{
		hidesAttachmentRows,
		isLastTurnUnsettled,
		hasMoreMessages,
	}: TurnAttachmentsOptions,
) => {
	"use memo";
	// The live row never hosts a pill.
	const withLiveRow = false;
	return getTurnAttachmentsByRow(
		assignTimelineRows(buildDisplayMessages(parsedMessages), withLiveRow),
		{ hidesAttachmentRows, isLastTurnUnsettled, hasMoreMessages },
	);
};
