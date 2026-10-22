import { buildDisplayMessages } from "./messageHelpers";
import { assignTimelineRows } from "./timelineRows";
import type { ParsedMessageEntry } from "./types";
import {
	getTurnAttachmentsByRow,
	type TurnAttachmentsOptions,
} from "./workingBlockGrouping";

/**
 * "use memo" keys this cache on the messages and destructured flags; inlined,
 * the call would share the timeline body's cache scope, which every stream
 * chunk invalidates. It builds no live row: only message rows host a pill.
 */
export const useTurnAttachmentsByRow = (
	parsedMessages: readonly ParsedMessageEntry[],
	{
		hidesAttachmentRows,
		isChatActive,
		isNextTurnStarting,
		isCompacting,
		hasMoreMessages,
	}: TurnAttachmentsOptions,
) => {
	"use memo";
	return getTurnAttachmentsByRow(
		assignTimelineRows(buildDisplayMessages(parsedMessages), false),
		{
			hidesAttachmentRows,
			isChatActive,
			isNextTurnStarting,
			isCompacting,
			hasMoreMessages,
		},
	);
};
