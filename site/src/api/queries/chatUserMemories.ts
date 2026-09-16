import { type QueryClient, queryOptions } from "react-query";
import { API } from "#/api/api";
import type * as TypesGen from "#/api/typesGenerated";

const chatUserMemoriesFamilyKey = ["chat-user-memories"] as const;

const chatUserMemoriesKey = (organizationId: string) =>
	[...chatUserMemoriesFamilyKey, organizationId] as const;

const chatPersonalMemorySettingsKey = [
	"chat-personal-memory-settings",
] as const;

export const chatUserMemories = (organizationId: string) =>
	queryOptions({
		queryKey: chatUserMemoriesKey(organizationId),
		queryFn: () => API.experimental.getChatUserMemories(organizationId),
		enabled: Boolean(organizationId),
	});

export const chatUserMemoryConsolidations = (organizationId: string) =>
	queryOptions({
		queryKey: [...chatUserMemoriesKey(organizationId), "consolidations"],
		queryFn: () =>
			API.experimental.getChatUserMemoryConsolidations(organizationId),
		enabled: Boolean(organizationId),
	});

export const chatPersonalMemorySettings = () =>
	queryOptions({
		queryKey: chatPersonalMemorySettingsKey,
		queryFn: () => API.getChatPersonalMemorySettings(),
	});

const invalidateChatUserMemories = (queryClient: QueryClient) =>
	queryClient.invalidateQueries({ queryKey: chatUserMemoriesFamilyKey });

export const createChatUserMemory = (queryClient: QueryClient) => ({
	mutationFn: (request: TypesGen.CreateChatUserMemoryRequest) =>
		API.experimental.createChatUserMemory(request),
	onSettled: () => invalidateChatUserMemories(queryClient),
});

export const updateChatUserMemory = (queryClient: QueryClient) => ({
	mutationFn: ({
		memoryId,
		request,
	}: {
		memoryId: string;
		request: TypesGen.UpdateChatUserMemoryRequest;
	}) => API.experimental.updateChatUserMemory(memoryId, request),
	onSettled: () => invalidateChatUserMemories(queryClient),
});

export const deleteChatUserMemory = (queryClient: QueryClient) => ({
	mutationFn: (memoryId: string) =>
		API.experimental.deleteChatUserMemory(memoryId),
	onSettled: () => invalidateChatUserMemories(queryClient),
});

export const updateChatPersonalMemorySettings = (queryClient: QueryClient) => ({
	mutationFn: (request: TypesGen.UpdateChatPersonalMemorySettingsRequest) =>
		API.updateChatPersonalMemorySettings(request),
	onSettled: () =>
		queryClient.invalidateQueries({ queryKey: chatPersonalMemorySettingsKey }),
});
