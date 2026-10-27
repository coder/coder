import { getDisplayMessageKey } from "./messageHelpers";
import type { ParsedMessageEntry } from "./types";

/** A timeline row for one durable parsed message. */
export type TimelineMessageRow = {
	type: "message";
	entry: ParsedMessageEntry;
	key: string;
	isLastMessage: boolean;
};

export type TimelineRow = TimelineMessageRow | { type: "live"; key: string };

/**
 * Durable rows keep their server IDs so prepending history never changes an
 * existing Item's identity. Merged read_file groups key off their newest
 * member, which pagination never changes. The live row is keyed independently
 * of which history pages are loaded.
 */
export const assignTimelineRows = (
	displayMessages: readonly ParsedMessageEntry[],
	hasLiveAssistant: boolean,
): readonly TimelineRow[] => {
	const rows = displayMessages.map(
		(entry, index): TimelineMessageRow => ({
			type: "message",
			entry,
			key: getDisplayMessageKey(entry),
			isLastMessage: index === displayMessages.length - 1,
		}),
	);
	if (!hasLiveAssistant) {
		return rows;
	}
	return [...rows, { type: "live", key: "live-assistant" }];
};
