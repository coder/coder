import { getDisplayMessageKey } from "./messageHelpers";
import type { ParsedMessageEntry } from "./types";

type TimelineMessageRow = {
	type: "message";
	entry: ParsedMessageEntry;
	key: string;
	isLastInAssistantChain: boolean;
	isLastMessage: boolean;
};

type TimelineRow = TimelineMessageRow | { type: "live"; key: string };

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
	const rows: TimelineMessageRow[] = [];

	for (const [index, entry] of displayMessages.entries()) {
		rows.push({
			type: "message",
			entry,
			key: getDisplayMessageKey(entry),
			isLastInAssistantChain: false,
			isLastMessage: index === displayMessages.length - 1,
		});
	}

	// Message actions only belong on the final message of a consecutive
	// assistant chain, so walk backwards and mark the ones a user message
	// (or the end of the transcript) follows.
	let nextVisibleIsUser = true;
	for (let i = rows.length - 1; i >= 0; i--) {
		const { message } = rows[i].entry;
		if (message.role === "system") {
			nextVisibleIsUser = true;
			continue;
		}
		if (message.role !== "user") {
			rows[i].isLastInAssistantChain = nextVisibleIsUser;
		}
		nextVisibleIsUser = message.role === "user";
	}

	if (!hasLiveAssistant) {
		return rows;
	}
	return [...rows, { type: "live", key: "live-assistant" }];
};
