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

const assistantSlotKey = (turnKey: string, slot: number): string =>
	`${turnKey}:assistant:${slot}`;

/**
 * Durable rows keep their server IDs so prepending history never changes an
 * existing Item's identity. Merged read_file groups key off the visible row
 * before them for the same reason. The live assistant uses a turn-local slot
 * until its durable message arrives.
 */
export const assignTimelineRows = (
	displayMessages: readonly ParsedMessageEntry[],
	hasLiveAssistant: boolean,
): readonly TimelineRow[] => {
	const rows: TimelineMessageRow[] = [];
	let turnKey: string | undefined;
	let assistantsInTurn = 0;

	for (const [index, entry] of displayMessages.entries()) {
		const { message } = entry;
		const key = getDisplayMessageKey(entry, displayMessages[index - 1]);
		if (message.role === "user") {
			turnKey = key;
			assistantsInTurn = 0;
		} else if (message.role === "assistant" && turnKey) {
			assistantsInTurn += 1;
		}
		rows.push({
			type: "message",
			entry,
			key,
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
	return [
		...rows,
		{
			type: "live",
			key: turnKey
				? assistantSlotKey(turnKey, assistantsInTurn)
				: "live-assistant",
		},
	];
};
