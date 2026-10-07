import { mutationOptions, type QueryClient, queryOptions } from "react-query";
import { API } from "#/api/api";
import type * as TypesGen from "#/api/typesGenerated";
import {
	chatEntitiesFamilyKey,
	invalidateChatListQueries,
	invalidateChatSearches,
} from "./chats";

export const chatProjectsKey = ["chat-projects"] as const;

/**
 * Lists the current user's chat projects across all organizations.
 * @public
 */
export const chatProjects = () =>
	queryOptions({
		queryKey: chatProjectsKey,
		queryFn: () => API.experimental.getChatProjects(),
	});

/**
 * Reads one project from the chatProjects() list, because the single-project
 * GET needs the organization ID. Data is undefined while the list loads and
 * null when the list has no project with projectId.
 */
export const chatProject = (projectId: string | undefined) =>
	queryOptions({
		...chatProjects(),
		select: (projects) =>
			projects.find((project) => project.id === projectId) ?? null,
	});

export const createChatProject = (queryClient: QueryClient) =>
	mutationOptions({
		mutationFn: ({
			organizationId,
			request,
		}: {
			organizationId: string;
			request: TypesGen.CreateChatProjectRequest;
		}) => API.experimental.createChatProject(organizationId, request),
		onSettled: () =>
			queryClient.invalidateQueries({ queryKey: chatProjectsKey }),
	});

export const updateChatProject = (queryClient: QueryClient) =>
	mutationOptions({
		mutationFn: ({
			project,
			request,
		}: {
			project: TypesGen.ChatProject;
			request: TypesGen.UpdateChatProjectRequest;
		}) =>
			API.experimental.updateChatProject(
				project.organization_id,
				project.id,
				request,
			),
		onSettled: () =>
			queryClient.invalidateQueries({ queryKey: chatProjectsKey }),
	});

export const deleteChatProject = (queryClient: QueryClient) =>
	mutationOptions({
		mutationFn: (project: TypesGen.ChatProject) =>
			API.experimental.deleteChatProject(project.organization_id, project.id),
		onSettled: () =>
			// Deleting a project deletes its chats.
			Promise.all([
				queryClient.invalidateQueries({ queryKey: chatProjectsKey }),
				invalidateChatListQueries(queryClient),
				invalidateChatSearches(queryClient),
				// Resetting, not invalidating, the chat entities lets open
				// routes find a deleted chat gone: invalidation keeps cached
				// data when the refetch 404s. Their nested queries (messages,
				// ACL, diffs, ...) would only refetch into 404s.
				queryClient.resetQueries({
					queryKey: chatEntitiesFamilyKey,
					predicate: ({ queryKey }) =>
						queryKey.length === chatEntitiesFamilyKey.length + 1,
				}),
			]),
	});
