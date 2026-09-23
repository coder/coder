import {
	mutationOptions,
	type QueryClient,
	queryOptions,
	skipToken,
} from "react-query";
import { API } from "#/api/api";
import type * as TypesGen from "#/api/typesGenerated";
import { invalidateChatListQueries } from "./chats";

const chatProjectsFamilyKey = ["chat-projects"] as const;

const chatProjectsKey = (organizationId: string | undefined) =>
	[...chatProjectsFamilyKey, organizationId] as const;

const chatProjectKey = (projectId: string | undefined) =>
	[...chatProjectsFamilyKey, "project", projectId] as const;

export const chatProjects = (organizationId: string | undefined) =>
	queryOptions({
		queryKey: chatProjectsKey(organizationId),
		queryFn: organizationId
			? () => API.experimental.getChatProjects(organizationId)
			: skipToken,
	});

export const chatProject = (projectId: string | undefined) =>
	queryOptions({
		queryKey: chatProjectKey(projectId),
		queryFn: projectId
			? () => API.experimental.getChatProject(projectId)
			: skipToken,
	});

export const createChatProject = (queryClient: QueryClient) =>
	mutationOptions({
		mutationFn: (request: TypesGen.CreateChatProjectRequest) =>
			API.experimental.createChatProject(request),
		onSettled: () =>
			queryClient.invalidateQueries({ queryKey: chatProjectsFamilyKey }),
	});

export const updateChatProject = (queryClient: QueryClient) =>
	mutationOptions({
		mutationFn: ({
			projectId,
			request,
		}: {
			projectId: string;
			request: TypesGen.UpdateChatProjectRequest;
		}) => API.experimental.updateChatProject(projectId, request),
		onSettled: (_data, _error, { projectId }) =>
			Promise.all([
				queryClient.invalidateQueries({ queryKey: chatProjectsFamilyKey }),
				queryClient.invalidateQueries({ queryKey: chatProjectKey(projectId) }),
			]),
	});

export const deleteChatProject = (queryClient: QueryClient) =>
	mutationOptions({
		mutationFn: (projectId: string) =>
			API.experimental.deleteChatProject(projectId),
		onSettled: () =>
			Promise.all([
				queryClient.invalidateQueries({ queryKey: chatProjectsFamilyKey }),
				invalidateChatListQueries(queryClient),
			]),
	});
