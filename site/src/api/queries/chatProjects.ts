import { type QueryClient, queryOptions } from "react-query";
import { API } from "#/api/api";
import type * as TypesGen from "#/api/typesGenerated";
import {
	chatProjectKey,
	chatProjectsFamilyKey,
	chatProjectsKey,
} from "./chatProjectsKeys";
import { invalidateChatListQueries, invalidateChatsByWorkspace } from "./chats";

export const chatProjects = (organizationId: string) =>
	queryOptions({
		queryKey: chatProjectsKey(organizationId),
		queryFn: () => API.experimental.getChatProjects(organizationId),
		enabled: Boolean(organizationId),
	});

export const chatProject = (projectId: string) =>
	queryOptions({
		queryKey: chatProjectKey(projectId),
		queryFn: () => API.experimental.getChatProject(projectId),
		enabled: Boolean(projectId),
	});

const invalidateChatProjects = (queryClient: QueryClient) =>
	queryClient.invalidateQueries({ queryKey: chatProjectsFamilyKey });

const invalidateProjectRelatedQueries = async (queryClient: QueryClient) => {
	await Promise.all([
		invalidateChatProjects(queryClient),
		invalidateChatListQueries(queryClient),
		invalidateChatsByWorkspace(queryClient),
	]);
};

export const createChatProject = (queryClient: QueryClient) => ({
	mutationFn: (request: TypesGen.CreateChatProjectRequest) =>
		API.experimental.createChatProject(request),
	onSettled: () => invalidateProjectRelatedQueries(queryClient),
});

export const updateChatProject = (queryClient: QueryClient) => ({
	mutationFn: ({
		projectId,
		request,
	}: {
		projectId: string;
		request: TypesGen.UpdateChatProjectRequest;
	}) => API.experimental.updateChatProject(projectId, request),
	onSettled: (
		_data: unknown,
		_error: unknown,
		{ projectId }: { projectId: string },
	) =>
		Promise.all([
			invalidateProjectRelatedQueries(queryClient),
			queryClient.invalidateQueries({ queryKey: chatProjectKey(projectId) }),
		]),
});

export const deleteChatProject = (queryClient: QueryClient) => ({
	mutationFn: (projectId: string) =>
		API.experimental.deleteChatProject(projectId),
	onSettled: () => invalidateProjectRelatedQueries(queryClient),
});
