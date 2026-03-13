import { API, watchChat } from "api/api";
import type * as TypesGen from "api/typesGenerated";
import type {
	ChatModelOption,
	ChatRuntime,
	ChatStreamEvent,
} from "modules/chat-shared";
import type {
	OneWayMessageEvent,
	OneWayWebSocket,
} from "utils/OneWayWebSocket";
import { createReconnectingWebSocket } from "utils/reconnectingWebSocket";

type BrowserChatStreamSocket = OneWayWebSocket<TypesGen.ServerSentEvent>;

interface BrowserChatRuntimeDeps {
	getChats: typeof API.getChats;
	getChat: typeof API.getChat;
	createChatMessage: typeof API.createChatMessage;
	getChatModels: typeof API.getChatModels;
	watchChat: typeof watchChat;
	createReconnectingWebSocket: typeof createReconnectingWebSocket;
}

const defaultDeps: BrowserChatRuntimeDeps = {
	getChats: API.getChats,
	getChat: API.getChat,
	createChatMessage: API.createChatMessage,
	getChatModels: API.getChatModels,
	watchChat,
	createReconnectingWebSocket,
};

const isChatStreamEvent = (
	data: unknown,
): data is ChatStreamEvent & Record<string, unknown> =>
	typeof data === "object" &&
	data !== null &&
	"type" in data &&
	typeof (data as Record<string, unknown>).type === "string";

const isChatStreamEventArray = (
	data: unknown,
): data is (ChatStreamEvent & Record<string, unknown>)[] =>
	Array.isArray(data) && data.every(isChatStreamEvent);

const toChatStreamEvents = (
	data: unknown,
): (ChatStreamEvent & Record<string, unknown>)[] => {
	if (isChatStreamEvent(data)) {
		return [data];
	}
	if (isChatStreamEventArray(data)) {
		return data;
	}
	return [];
};

const toChatModelOptions = (
	response: TypesGen.ChatModelsResponse,
): ChatModelOption[] => {
	return response.providers.flatMap((provider) => {
		if (!provider.available) {
			return [];
		}

		return provider.models.map((model) => ({
			id: model.id,
			provider: model.provider,
			model: model.model,
			displayName: model.display_name,
		}));
	});
};

const updateLastDurableMessageId = (
	current: number | undefined,
	messageId: number | undefined,
): number | undefined => {
	if (messageId === undefined) {
		return current;
	}
	if (current === undefined) {
		return messageId;
	}
	return Math.max(current, messageId);
};

export const createBrowserChatRuntime = (
	deps: BrowserChatRuntimeDeps = defaultDeps,
): ChatRuntime => {
	return {
		async listChats(input) {
			const chats = await deps.getChats({
				limit: input?.limit,
				offset: input?.offset,
			});

			if (input?.archived === undefined) {
				return chats;
			}

			// The REST endpoint does not yet support archived filtering, so the
			// browser runtime must post-filter the returned chat list client-side.
			return chats.filter((chat) => chat.archived === input.archived);
		},

		getChat(chatId) {
			return deps.getChat(chatId);
		},

		async sendMessage({ chatId, message, model, parentMessageId }) {
			if (parentMessageId !== undefined) {
				console.warn(
					"browserChatRuntime.sendMessage received a parentMessageId, but the browser chat API does not support threaded replies yet.",
				);
			}

			return deps.createChatMessage(chatId, {
				content: [{ type: "text", text: message }],
				model_config_id: model,
			});
		},

		async listModels() {
			const response = await deps.getChatModels();
			return toChatModelOptions(response);
		},

		subscribeToChat({ chatId, afterMessageId }, onEvent) {
			let disposed = false;
			let lastDurableMessageId = afterMessageId;

			const handleMessage = (
				payload: OneWayMessageEvent<TypesGen.ServerSentEvent>,
			) => {
				// Guard against socket events that race with dispose().
				if (disposed) {
					return;
				}
				if (payload.parseError || !payload.parsedMessage) {
					return;
				}
				if (payload.parsedMessage.type !== "data") {
					return;
				}

				const streamEvents = toChatStreamEvents(payload.parsedMessage.data);
				for (const streamEvent of streamEvents) {
					if (disposed) {
						return;
					}

					onEvent(streamEvent);
					if (streamEvent.type !== "message") {
						continue;
					}

					lastDurableMessageId = updateLastDurableMessageId(
						lastDurableMessageId,
						streamEvent.message.id,
					);
				}
			};

			const disposeSocket =
				deps.createReconnectingWebSocket<BrowserChatStreamSocket>({
					connect() {
						const socket = deps.watchChat(chatId, lastDurableMessageId);
						socket.addEventListener("message", handleMessage);
						return socket;
					},
				});

			return {
				dispose() {
					disposed = true;
					disposeSocket();
				},
			};
		},
	};
};
