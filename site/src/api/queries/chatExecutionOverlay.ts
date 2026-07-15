import type { QueryClient } from "react-query";
import type * as TypesGen from "#/api/typesGenerated";

export type ChatExecutionOwnershipToken = symbol;

type ChatExecutionProjection = TypesGen.Chat & {
	readonly action_required?: TypesGen.ChatStreamActionRequired;
};

const executionOwners = new WeakMap<
	QueryClient,
	Map<string, ChatExecutionOwnershipToken>
>();

const getExecutionOwners = (queryClient: QueryClient) => {
	let owners = executionOwners.get(queryClient);
	if (!owners) {
		owners = new Map();
		executionOwners.set(queryClient, owners);
	}
	return owners;
};

export const claimChatExecutionOverlay = (
	queryClient: QueryClient,
	chatID: string,
): ChatExecutionOwnershipToken => {
	const token = Symbol(chatID);
	getExecutionOwners(queryClient).set(chatID, token);
	return token;
};

export const releaseChatExecutionOverlay = (
	queryClient: QueryClient,
	chatID: string,
	token: ChatExecutionOwnershipToken,
): void => {
	const owners = executionOwners.get(queryClient);
	if (owners?.get(chatID) === token) {
		owners.delete(chatID);
	}
};

export const preserveChatExecutionOverlay = <T extends ChatExecutionProjection>(
	queryClient: QueryClient,
	chat: T,
): T => {
	if (!executionOwners.get(queryClient)?.has(chat.id)) {
		return chat;
	}
	const current = queryClient.getQueryData<ChatExecutionProjection>([
		"chats",
		"detail",
		chat.id,
	]);
	if (!current) {
		return chat;
	}
	return {
		...chat,
		status: current.status,
		last_error: current.last_error,
		action_required: current.action_required,
	};
};
