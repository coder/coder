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
	queuedMessages,
}: {
	currentData: InfiniteData<TypesGen.ChatMessagesResponse> | undefined;
	editedMessageId: number;
	replacementMessage?: TypesGen.ChatMessage;
	queuedMessages?: readonly TypesGen.ChatQueuedMessage[];
}): InfiniteData<TypesGen.ChatMessagesResponse> | undefined => {
	if (!currentData?.pages?.length) {
		return currentData;
	}

	const truncatedPages = currentData.pages.map((page, pageIndex) => {
		const truncatedMessages = page.messages.filter(
			(message) => message.id < editedMessageId,
		);
		const nextPage = {
			...page,
			...(pageIndex === 0 && queuedMessages !== undefined
				? { queued_messages: queuedMessages }
				: {}),
		};
		if (pageIndex !== 0 || !replacementMessage) {
			return { ...nextPage, messages: truncatedMessages };
		}
		return {
			...nextPage,
			messages: upsertFirstPageMessage(truncatedMessages, replacementMessage),
		};
	});

	return {
		...currentData,
		pages: truncatedPages,
	};
};

export const reconcileEditedMessageInCache = ({
	currentData,
	optimisticMessageId,
	responseMessage,
}: {
	currentData: InfiniteData<TypesGen.ChatMessagesResponse> | undefined;
	optimisticMessageId: number;
	responseMessage: TypesGen.ChatMessage;
}): InfiniteData<TypesGen.ChatMessagesResponse> | undefined => {
	if (!currentData?.pages?.length) {
		return currentData;
	}

	const replacedPages = currentData.pages.map((page, pageIndex) => {
		const preservedMessages = page.messages.filter(
			(message) =>
				message.id !== optimisticMessageId && message.id !== responseMessage.id,
		);
		if (pageIndex !== 0) {
			return { ...page, messages: preservedMessages };
		}
		return {
			...page,
			messages: upsertFirstPageMessage(preservedMessages, responseMessage),
		};
	});

	return {
		...currentData,
		pages: replacedPages,
	};
};
