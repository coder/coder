import type * as TypesGen from "api/typesGenerated";
import type {
	ChatDetail,
	ChatMessage,
	ChatModelOption,
	ChatPreferenceStore,
	ChatPreferences,
	ChatQueuedMessage,
	ChatRuntime,
	ChatStatus,
	ChatStreamEvent,
	ChatSummary,
	SendMessageResult,
} from "modules/chat-shared";
import { describe, expect, it } from "vitest";

describe("chatRuntime shared types", () => {
	it("exposes headless runtime contracts that align with generated chat types", () => {
		const summary = {} as TypesGen.Chat as ChatSummary;
		const detail = {} as TypesGen.ChatWithMessages as ChatDetail;
		const sendResult =
			{} as TypesGen.CreateChatMessageResponse as SendMessageResult;
		const message = {} as TypesGen.ChatMessage as ChatMessage;
		const queuedMessage = {} as TypesGen.ChatQueuedMessage as ChatQueuedMessage;
		const status = "running" as ChatStatus;
		const model: ChatModelOption = {
			id: "openai:gpt-4o",
			provider: "openai",
			model: "gpt-4o",
			displayName: "GPT-4o",
		};
		const preferences: ChatPreferences = {
			get: (_key, fallback) => fallback,
			set: () => undefined,
			subscribe: () => () => undefined,
		};
		const preferenceStore: ChatPreferenceStore = preferences;
		const runtime: ChatRuntime = {
			listChats: async () => [summary],
			getChat: async () => detail,
			sendMessage: async () => sendResult,
			listModels: async () => [model],
			subscribeToChat: () => ({
				dispose() {
					return undefined;
				},
			}),
		};
		const event: ChatStreamEvent = {
			type: "status",
			chat_id: "chat-1",
			status: { status },
		};

		expect(
			runtime.subscribeToChat({ chatId: "chat-1" }, () => undefined),
		).toEqual({ dispose: expect.any(Function) });
		expect(preferenceStore.get("chat.model", model.id)).toBe(model.id);
		expect(event.status.status).toBe("running");
		expect([message, queuedMessage]).toHaveLength(2);
	});
});
