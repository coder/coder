import type { InfiniteData } from "react-query";
import type * as TypesGen from "#/api/typesGenerated";
import { chatMessagesEqualByValue } from "./chatStore";

const sortMessagesNewestFirst = (
	messages: Iterable<TypesGen.ChatMessage>,
): TypesGen.ChatMessage[] => {
	const sorted = Array.from(messages);
	sorted.sort((a, b) => b.id - a.id);
	return sorted;
};

const dedupeMessages = (
	messages: readonly TypesGen.ChatMessage[],
): TypesGen.ChatMessage[] => {
	const byID = new Map<number, TypesGen.ChatMessage>();
	for (const message of messages) {
		byID.set(message.id, message);
	}
	return Array.from(byID.values());
};

export const mergeMessagesIntoInfiniteCache = (
	prev: InfiniteData<TypesGen.ChatMessagesResponse> | undefined,
	incoming: readonly TypesGen.ChatMessage[],
): InfiniteData<TypesGen.ChatMessagesResponse> => {
	if (!prev || prev.pages.length === 0) {
		return {
			pageParams: [undefined],
			pages: [
				{
					messages: sortMessagesNewestFirst(dedupeMessages(incoming)),
					queued_messages: [],
					has_more: true,
				},
			],
		};
	}

	if (incoming.length === 0) {
		return prev;
	}

	const firstPage = prev.pages[0];
	const existingByID = new Map(firstPage.messages.map((m) => [m.id, m]));

	let changed = false;
	for (const msg of incoming) {
		const existing = existingByID.get(msg.id);
		if (!existing || !chatMessagesEqualByValue(existing, msg)) {
			changed = true;
			existingByID.set(msg.id, msg);
		}
	}

	if (!changed) {
		return prev;
	}

	return {
		...prev,
		pages: [
			{
				...firstPage,
				messages: sortMessagesNewestFirst(existingByID.values()),
			},
			...prev.pages.slice(1),
		],
	};
};
