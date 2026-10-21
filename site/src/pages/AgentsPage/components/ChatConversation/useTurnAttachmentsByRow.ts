import { buildDisplayMessages } from "./messageHelpers";
import { assignTimelineRows } from "./timelineRows";
import type { ParsedMessageEntry } from "./types";
import {
	getTurnAttachmentsByRow,
	type TurnAttachmentsOptions,
} from "./workingBlockGrouping";

/**
 * "use memo" gives this hook its own cache, keyed on the flags rather than the
 * caller's options object: inlined in the timeline body the call is not
 * memoized, so memoized host rows would re-render on every stream chunk.
 */
export const useTurnAttachmentsByRow = (
	parsedMessages: readonly ParsedMessageEntry[],
	{ hidesAttachmentRows, isLastTurnActive }: TurnAttachmentsOptions,
) => {
	"use memo";
	return getTurnAttachmentsByRow(
		assignTimelineRows(buildDisplayMessages(parsedMessages), false),
		{ hidesAttachmentRows, isLastTurnActive },
	);
};
