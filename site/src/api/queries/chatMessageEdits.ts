import type { InfiniteData } from "react-query";
import type * as TypesGen from "#/api/typesGenerated";

const buildOptimisticEditedContent = ({
	requestContent,
	originalMessage,
	attachmentMediaTypes,
}: {
	requestContent: readonly TypesGen.ChatInputPart[];
	originalMessage: TypesGen.ChatMessage;
	attachmentMediaTypes?: ReadonlyMap<string, string>;
}): readonly TypesGen.ChatMessagePart[] => {
	const existingFilePartsByID = new Map<string, TypesGen.ChatFilePart>();
	for (const part of originalMessage.content ?? []) {
		if (part.type === "file" && part.file_id) {
			existingFilePartsByID.set(part.file_id, part);
		}
	}

	return requestContent.map((part): TypesGen.ChatMessagePart => {
		if (part.type === "text") {
			return { type: "text", text: part.text ?? "" };
		}
		if (part.type === "file-reference") {
			return {
				type: "file-reference",
				file_name: part.file_name ?? "",
				start_line: part.start_line ?? 1,
				end_line: part.end_line ?? 1,
				content: part.content ?? "",
			};
		}
		const fileId = part.file_id ?? "";
		return (
			existingFilePartsByID.get(fileId) ?? {
				type: "file",
				file_id: part.file_id,
				media_type:
					attachmentMediaTypes?.get(fileId) ?? "application/octet-stream",
			}
		);
	});
};

export const buildOptimisticEditedMessage = ({
	requestContent,
	originalMessage,
	attachmentMediaTypes,
}: {
	requestContent: readonly TypesGen.ChatInputPart[];
	originalMessage: TypesGen.ChatMessage;
	attachmentMediaTypes?: ReadonlyMap<string, string>;
}): TypesGen.ChatMessage => ({
	...originalMessage,
	content: buildOptimisticEditedContent({
		requestContent,
		originalMessage,
		attachmentMediaTypes,
	}),
});

const sortMessagesDescending = (
	messages: readonly TypesGen.ChatMessage[],
): TypesGen.ChatMessage[] => [...messages].sort((a, b) => b.id - a.id);

const upsertFirstPageMessage = (
	messages: readonly TypesGen.ChatMessage[],
	message: TypesGen.ChatMessage,
): TypesGen.ChatMessage[] => {
	const byID = new Map(
		messages.map((existingMessage) => [existingMessage.id, existingMessage]),
	);
	byID.set(message.id, message);
	return sortMessagesDescending(Array.from(byID.values()));
};

export const projectEditedConversationIntoCache = ({
	currentData,
	editedMessageId,
	replacementMessage,
}: {
	currentData: InfiniteData<TypesGen.ChatMessagesResponse> | undefined;
	editedMessageId: number;
	replacementMessage?: TypesGen.ChatMessage;
}): InfiniteData<TypesGen.ChatMessagesResponse> | undefined => {
	if (!currentData?.pages?.length) {
		return currentData;
	}

	// The queue is deliberately untouched. The edit clears the server queue,
	// but hiding it optimistically is a read-time suppression marker: a cache
	// clear here would need a rollback in `onError`, and that rollback would
	// clobber a `queue_update` delivered while the edit was in flight.
	const truncatedPages = currentData.pages.map((page, pageIndex) => {
		const truncatedMessages = page.messages.filter(
			(message) => message.id < editedMessageId,
		);
		if (pageIndex !== 0 || !replacementMessage) {
			return { ...page, messages: truncatedMessages };
		}
		return {
			...page,
			messages: upsertFirstPageMessage(truncatedMessages, replacementMessage),
		};
	});

	return {
		...currentData,
		pages: truncatedPages,
	};
};

/**
 * Restores the pre-edit conversation after a failed edit, keeping messages that
 * landed while the mutation was in flight.
 *
 * A plain snapshot restore would drop socket-delivered messages committed after
 * `onMutate` read the snapshot, so anything present now but absent from the
 * snapshot is re-inserted into page 0. IDs the snapshot already holds keep the
 * snapshot's copy, which is what undoes the optimistic replacement.
 *
 * The queue is never restored. The edit's optimistic hide is a read-time
 * suppression marker rather than a cache write, so page 0 already carries
 * whatever the server last reported, including a `queue_update` delivered
 * mid-flight. Reinstating the snapshot's copy would clobber exactly that.
 */
export const restoreEditedConversationInCache = ({
	currentData,
	previousData,
}: {
	currentData: InfiniteData<TypesGen.ChatMessagesResponse> | undefined;
	previousData: InfiniteData<TypesGen.ChatMessagesResponse>;
}): InfiniteData<TypesGen.ChatMessagesResponse> => {
	if (!previousData.pages.length) {
		return previousData;
	}
	const currentQueuedMessages = currentData?.pages?.[0]?.queued_messages;
	const previousIDs = new Set(
		previousData.pages.flatMap((page) =>
			page.messages.map((message) => message.id),
		),
	);
	const arrivedDuringFlight = (currentData?.pages ?? []).flatMap((page) =>
		page.messages.filter((message) => !previousIDs.has(message.id)),
	);
	let messages = previousData.pages[0].messages;
	for (const message of arrivedDuringFlight) {
		messages = upsertFirstPageMessage(messages, message);
	}
	const queuedMessages =
		currentQueuedMessages ?? previousData.pages[0].queued_messages;
	if (
		arrivedDuringFlight.length === 0 &&
		queuedMessages === previousData.pages[0].queued_messages
	) {
		return previousData;
	}
	return {
		...previousData,
		pages: [
			{
				...previousData.pages[0],
				messages,
				queued_messages: queuedMessages,
			},
			...previousData.pages.slice(1),
		],
	};
};

export const reconcileEditedMessageInCache = ({
	currentData,
	optimisticMessageId,
	responseMessages,
	deletedMessageIds,
}: {
	currentData: InfiniteData<TypesGen.ChatMessagesResponse> | undefined;
	optimisticMessageId: number;
	responseMessages: readonly TypesGen.ChatMessage[];
	deletedMessageIds?: readonly number[];
}): InfiniteData<TypesGen.ChatMessagesResponse> | undefined => {
	if (!currentData?.pages?.length || responseMessages.length === 0) {
		return currentData;
	}

	const responseIDs = new Set(responseMessages.map((message) => message.id));
	const deletedIDs = new Set(deletedMessageIds ?? []);
	const replacedPages = currentData.pages.map((page, pageIndex) => {
		const preservedMessages = page.messages.filter(
			(message) =>
				message.id !== optimisticMessageId &&
				!responseIDs.has(message.id) &&
				!deletedIDs.has(message.id),
		);
		if (pageIndex !== 0) {
			return { ...page, messages: preservedMessages };
		}
		let messages = preservedMessages;
		for (const responseMessage of responseMessages) {
			messages = upsertFirstPageMessage(messages, responseMessage);
		}
		return { ...page, messages };
	});

	return {
		...currentData,
		pages: replacedPages,
	};
};
