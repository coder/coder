import type * as TypesGen from "api/typesGenerated";

export type ChatSummary = TypesGen.Chat;
export type ChatDetail = TypesGen.ChatWithMessages;
export type SendMessageResult = TypesGen.CreateChatMessageResponse;
export type ChatStatus = TypesGen.ChatStatus;
export type ChatMessage = TypesGen.ChatMessage;
export type ChatQueuedMessage = TypesGen.ChatQueuedMessage;

export type ChatModelOption = {
	id: string;
	provider: string;
	model: string;
	displayName: string;
};

export interface ChatRuntime {
	listChats(input?: {
		archived?: boolean;
		limit?: number;
		offset?: number;
	}): Promise<readonly ChatSummary[]>;
	getChat(chatId: string): Promise<ChatDetail>;
	sendMessage(input: {
		chatId: string;
		message: string;
		model?: string;
		parentMessageId?: number;
	}): Promise<SendMessageResult>;
	listModels(): Promise<readonly ChatModelOption[]>;
	subscribeToChat(
		input: { chatId: string; afterMessageId?: number },
		onEvent: (event: ChatStreamEvent) => void,
	): { dispose(): void };
}

export interface ChatPreferences {
	get<T>(key: string, fallback: T): T;
	set<T>(key: string, value: T): void;
	subscribe?(key: string, cb: () => void): () => void;
}

export interface ChatPreferenceStore extends ChatPreferences {}

export type ChatStreamEvent =
	| {
			type: "message_part";
			chat_id?: string;
			message_part: { part: Record<string, unknown> };
	  }
	| {
			type: "message";
			chat_id?: string;
			message: ChatMessage;
	  }
	| {
			type: "queue_update";
			chat_id?: string;
			queued_messages: readonly ChatQueuedMessage[];
	  }
	| {
			type: "status";
			chat_id?: string;
			status: { status: ChatStatus };
	  }
	| {
			type: "error";
			chat_id?: string;
			error: { message: string };
	  }
	| {
			type: "retry";
			chat_id?: string;
			retry: { attempt: number; error: string };
	  };
