import { type QueryClient, queryOptions } from "react-query";
import { API } from "#/api/api";
import type * as TypesGen from "#/api/typesGenerated";
import { chatProjectMemoriesKey } from "./chatProjectsKeys";

export const chatProjectMemories = (projectId: string) =>
	queryOptions({
		queryKey: chatProjectMemoriesKey(projectId),
		queryFn: () => API.experimental.getChatProjectMemories(projectId),
		enabled: Boolean(projectId),
	});

const invalidateChatProjectMemories = (
	queryClient: QueryClient,
	projectId: string,
) =>
	queryClient.invalidateQueries({
		queryKey: chatProjectMemoriesKey(projectId),
	});

export const createChatProjectMemory = (queryClient: QueryClient) => ({
	mutationFn: ({
		projectId,
		request,
	}: {
		projectId: string;
		request: TypesGen.CreateChatProjectMemoryRequest;
	}) => API.experimental.createChatProjectMemory(projectId, request),
	onSettled: (
		_data: unknown,
		_error: unknown,
		{ projectId }: { projectId: string },
	) => invalidateChatProjectMemories(queryClient, projectId),
});

export const updateChatProjectMemory = (queryClient: QueryClient) => ({
	mutationFn: ({
		projectId,
		memoryId,
		request,
	}: {
		projectId: string;
		memoryId: string;
		request: TypesGen.UpdateChatProjectMemoryRequest;
	}) => API.experimental.updateChatProjectMemory(projectId, memoryId, request),
	onSettled: (
		_data: unknown,
		_error: unknown,
		{ projectId }: { projectId: string },
	) => invalidateChatProjectMemories(queryClient, projectId),
});

export const deleteChatProjectMemory = (queryClient: QueryClient) => ({
	mutationFn: ({
		projectId,
		memoryId,
	}: {
		projectId: string;
		memoryId: string;
	}) => API.experimental.deleteChatProjectMemory(projectId, memoryId),
	onSettled: (
		_data: unknown,
		_error: unknown,
		{ projectId }: { projectId: string },
	) => invalidateChatProjectMemories(queryClient, projectId),
});
