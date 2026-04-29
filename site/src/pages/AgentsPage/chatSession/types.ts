import type { InfiniteData, QueryClient } from "react-query";
import type * as TypesGen from "#/api/typesGenerated";

export type ChatLifecycleMode = "background" | "foreground" | "inactive";

export type ChatViewportAnchor = {
	readonly messageId: number;
	readonly offsetTop: number;
	readonly newestMessageIdAtCapture: number | undefined;
};

export type ChatSessionSnapshot = {
	readonly lifecycleMode: ChatLifecycleMode;
	readonly followMode: boolean;
	readonly viewportAnchor: ChatViewportAnchor | null;
	readonly hasNewOffscreenContent: boolean;
	readonly lastVisibleAt?: number;
	readonly backgroundedAt?: number;
};

export type HydrateFromRestParams = {
	readonly chatMessages: readonly TypesGen.ChatMessage[] | undefined;
	readonly chatRecord: TypesGen.Chat | undefined;
	readonly chatMessagesData:
		| InfiniteData<TypesGen.ChatMessagesResponse>
		| TypesGen.ChatMessagesResponse
		| undefined;
	readonly chatQueuedMessages:
		| readonly TypesGen.ChatQueuedMessage[]
		| undefined;
};

export type StreamParams = {
	readonly mode: Extract<ChatLifecycleMode, "background" | "foreground">;
	readonly markRead: boolean;
	readonly intentionalModeSwitch?: boolean;
};

export type EnterForegroundParams = {
	readonly now?: number;
};

export type ReleaseVisibleParams = {
	readonly now?: number;
};

export type ChatSessionRuntimeDeps = {
	readonly queryClient: QueryClient;
	readonly setChatErrorReason: (chatId: string, reason: string) => void;
	readonly clearChatErrorReason: (chatId: string) => void;
};

export type ChatSessionManagerRuntimeDeps = ChatSessionRuntimeDeps;
