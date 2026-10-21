import { buildDisplayMessages } from "./messageHelpers";
import { assignTimelineRows } from "./timelineRows";
import type { ParsedMessageEntry } from "./types";
import { getTurnAttachmentsByRow } from "./workingBlockGrouping";

/**
 * Turn attachments keyed by host row. "use memo" gives this hook its own
 * cache: inlined in the timeline body the call is not memoized, so memoized
 * host rows would re-render on every stream chunk.
 */
export const useTurnAttachmentsByRow = (
	parsedMessages: readonly ParsedMessageEntry[],
	hidesAttachmentRows: boolean,
	isLastTurnActive: boolean,
) => {
	"use memo";
	return getTurnAttachmentsByRow(
		assignTimelineRows(buildDisplayMessages(parsedMessages), false),
		{ hidesAttachmentRows, isLastTurnActive },
	);
};
