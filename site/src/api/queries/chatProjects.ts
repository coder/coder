import { mutationOptions, type QueryClient, queryOptions } from "react-query";
import { API } from "#/api/api";
import type * as TypesGen from "#/api/typesGenerated";
import {
	chatEntitiesFamilyKey,
	chatEntityKey,
	chatProjectsKey,
	invalidateChatListQueries,
	invalidateChatSearches,
} from "./chats";

/**
 * Lists the chat projects the current user owns or that are shared with
 * them, across all organizations.
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
		// The reset runs here rather than in onSuccess, which callers replace.
		mutationFn: async (project: TypesGen.ChatProject) => {
			await API.experimental.deleteChatProject(
				project.organization_id,
				project.id,
			);
			resetDeletedProjectChats(queryClient, project.id);
		},
		onSettled: () =>
			Promise.all([
				queryClient.invalidateQueries({ queryKey: chatProjectsKey }),
				invalidateChatListQueries(queryClient),
				invalidateChatSearches(queryClient),
			]),
	});

// Resets rather than invalidates: invalidation keeps cached data when the
// refetch 404s, so open routes would keep rendering the chats.
const resetDeletedProjectChats = (
	queryClient: QueryClient,
	projectId: string,
) => {
	const chats = queryClient
		.getQueriesData<TypesGen.Chat>({
			queryKey: chatEntitiesFamilyKey,
			predicate: ({ queryKey }) =>
				queryKey.length === chatEntitiesFamilyKey.length + 1,
		})
		.flatMap(([, chat]) => (chat ? [chat] : []));
	const deleted = new Set(
		chats.filter((chat) => chat.project_id === projectId).map((c) => c.id),
	);
	for (const chat of chats) {
		if (chat.root_chat_id && deleted.has(chat.root_chat_id)) {
			deleted.add(chat.id);
		}
	}
	for (const id of deleted) {
		void queryClient.resetQueries({ queryKey: chatEntityKey(id), exact: true });
	}
};
