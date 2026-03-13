import { useMemo } from "react";
import {
	buildParsedMessageSections,
	buildStreamTools,
	type MergedTool,
	type ParsedMessageSection,
	parseMessagesWithMergedTools,
	type RenderBlock,
} from "../core";
import { useChatStoreSnapshot } from "./ChatRuntimeProvider";

export type UseChatMessagesResult = {
	sections: readonly ParsedMessageSection[];
	queuedMessages: ReturnType<typeof useChatStoreSnapshot>["queuedMessages"];
	streamBlocks: readonly RenderBlock[];
	streamTools: readonly MergedTool[];
	isStreaming: boolean;
};

/** @public Reads parsed shared chat messages for rendering. */
export const useChatMessages = (): UseChatMessagesResult => {
	const snapshot = useChatStoreSnapshot();

	const orderedMessages = useMemo(
		() =>
			snapshot.orderedMessageIDs
				.map((messageID) => snapshot.messagesByID.get(messageID))
				.filter((message): message is NonNullable<typeof message> =>
					Boolean(message),
				),
		[snapshot.messagesByID, snapshot.orderedMessageIDs],
	);

	const sections = useMemo(() => {
		const parsedMessages = parseMessagesWithMergedTools(orderedMessages);
		return buildParsedMessageSections(parsedMessages);
	}, [orderedMessages]);

	const streamBlocks = snapshot.streamState?.blocks ?? [];
	const streamTools = useMemo(
		() => buildStreamTools(snapshot.streamState),
		[snapshot.streamState],
	);
	const isStreaming =
		snapshot.chatStatus === "running" || snapshot.streamState !== null;

	return {
		sections,
		queuedMessages: snapshot.queuedMessages,
		streamBlocks,
		streamTools,
		isStreaming,
	};
};
