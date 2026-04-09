import type { InfiniteData } from "react-query";
import type * as TypesGen from "#/api/typesGenerated";

const buildOptimisticEditedContent = ({
	requestContent,
	originalMessage,
}: {
	requestContent: readonly TypesGen.ChatInputPart[];
	originalMessage: TypesGen.ChatMessage;
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
		return (
			existingFilePartsByID.get(part.file_id ?? "") ?? {
				type: "file",
				file_id: part.file_id,
				media_type: "application/octet-stream",
			}
		);
	});
};

export const buildOptimisticEditedMessage = ({
	requestContent,
	originalMessage,
}: {
	requestContent: readonly TypesGen.ChatInputPart[];
	originalMessage: TypesGen.ChatMessage;
}): TypesGen.ChatMessage => ({
	...originalMessage,
	content: buildOptimisticEditedContent({ requestContent, originalMessage }),
});

export const projectEditedConversationIntoCache = (
	currentData: InfiniteData<TypesGen.ChatMessagesResponse> | undefined,
	editedMessageId: number,
	replacementMessage?: TypesGen.ChatMessage,
): InfiniteData<TypesGen.ChatMessagesResponse> | undefined => {
	if (!currentData?.pages?.length) {
		return currentData;
	}

	const truncatedPages = currentData.pages.map((page, pageIndex) => {
		const truncatedMessages = page.messages.filter(
			(message) => message.id < editedMessageId,
		);
		if (pageIndex !== 0 || !replacementMessage) {
			return { ...page, messages: truncatedMessages };
		}
		const nextMessages = [replacementMessage, ...truncatedMessages];
		nextMessages.sort((a, b) => b.id - a.id);
		return {
			...page,
			messages: nextMessages,
		};
	});

	return {
		...currentData,
		pages: truncatedPages,
	};
};
