import { type QueryClient, queryOptions } from "react-query";
import { API } from "#/api/api";
import type * as TypesGen from "#/api/typesGenerated";

const chatPersonalMemorySettingsKey = [
	"chat-personal-memory-settings",
] as const;

export const chatPersonalMemorySettings = () =>
	queryOptions({
		queryKey: chatPersonalMemorySettingsKey,
		queryFn: () => API.getChatPersonalMemorySettings(),
	});

export const updateChatPersonalMemorySettings = (queryClient: QueryClient) => ({
	mutationFn: (request: TypesGen.UpdateChatPersonalMemorySettingsRequest) =>
		API.updateChatPersonalMemorySettings(request),
	onSettled: () =>
		queryClient.invalidateQueries({ queryKey: chatPersonalMemorySettingsKey }),
});
