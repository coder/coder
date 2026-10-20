import type { InfiniteData } from "react-query";
import type * as TypesGen from "#/api/typesGenerated";

/** Local metadata for an attachment the server has not echoed back yet. */
type OptimisticAttachment = {
	mediaType: string;
	name: string;
	size: number;
};

const buildOptimisticEditedContent = ({
	requestContent,
	originalMessage,
	pendingAttachments,
}: {
	requestContent: readonly TypesGen.ChatInputPart[];
	originalMessage: TypesGen.ChatMessage;
	pendingAttachments?: ReadonlyMap<string, OptimisticAttachment>;
}): readonly TypesGen.ChatMessagePart[] => {
	const existingFilePartsByID = new Map<string, TypesGen.ChatFilePart>();
	const existingWorkspaceAttachmentsByID = new Map<
		string,
		TypesGen.ChatWorkspaceAttachmentPart
	>();
	for (const part of originalMessage.content ?? []) {
		if (part.type === "file" && part.file_id) {
			existingFilePartsByID.set(part.file_id, part);
		}
		if (part.type === "workspace-attachment") {
			existingWorkspaceAttachmentsByID.set(part.file_id, part);
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
		if (part.type === "workspace-file-reference") {
			return {
				type: "workspace-file-reference",
				workspace_file_path: part.workspace_file_path ?? "",
				workspace_file_name: part.workspace_file_name ?? "",
				workspace_file_size: part.workspace_file_size ?? 0,
				workspace_file_media_type: part.workspace_file_media_type,
				workspace_file_workspace_id: part.workspace_file_workspace_id ?? "",
			};
		}
		const fileId = part.file_id ?? "";
		if (part.type === "workspace-attachment") {
			const pending = pendingAttachments?.get(fileId);
			return (
				existingWorkspaceAttachmentsByID.get(fileId) ?? {
					type: "workspace-attachment",
					file_id: fileId,
					name: pending?.name,
					size: pending?.size,
					media_type: pending?.mediaType ?? "application/octet-stream",
				}
			);
		}
		return (
			existingFilePartsByID.get(fileId) ?? {
				type: "file",
				file_id: part.file_id,
				media_type:
					pendingAttachments?.get(fileId)?.mediaType ??
					"application/octet-stream",
			}
		);
	});
};

export const buildOptimisticEditedMessage = ({
	requestContent,
	originalMessage,
	pendingAttachments,
}: {
	requestContent: readonly TypesGen.ChatInputPart[];
	originalMessage: TypesGen.ChatMessage;
	pendingAttachments?: ReadonlyMap<string, OptimisticAttachment>;
}): TypesGen.ChatMessage => ({
	...originalMessage,
	content: buildOptimisticEditedContent({
		requestContent,
		originalMessage,
		pendingAttachments,
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
