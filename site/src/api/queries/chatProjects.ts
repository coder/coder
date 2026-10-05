import { mutationOptions, type QueryClient, queryOptions } from "react-query";
import { API } from "#/api/api";
import type * as TypesGen from "#/api/typesGenerated";
import { invalidateChatListQueries } from "./chats";

export const chatProjectsKey = ["chat-projects"] as const;

/** Lists the current user's chat projects across all organizations. */
export const chatProjects = () =>
	queryOptions({
		queryKey: chatProjectsKey,
		queryFn: () => API.experimental.getChatProjects(),
	});

/**
 * Finds one project in the project list. Project reads are scoped to the
 * project's organization, and the list is already loaded for the sidebar, so
 * a page that only knows the project ID reads it from there. The result is
 * null once the list has loaded without the project.
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
			Promise.all([
				queryClient.invalidateQueries({ queryKey: chatProjectsKey }),
				invalidateChatListQueries(queryClient),
			]),
	});

export const chatProjectInstructionsKey = (projectId: string) =>
	[...chatProjectsKey, projectId, "instructions"] as const;

export const chatProjectInstructions = (project: TypesGen.ChatProject) =>
	queryOptions({
		queryKey: chatProjectInstructionsKey(project.id),
		queryFn: () =>
			API.experimental.getChatProjectInstructions(
				project.organization_id,
				project.id,
			),
		// Instructions are shared by every editor of the project, so refresh
		// them when the tab regains focus to avoid editing stale text.
		refetchOnWindowFocus: true,
	});

export const updateChatProjectInstructions = (
	queryClient: QueryClient,
	project: TypesGen.ChatProject,
) =>
	mutationOptions({
		mutationFn: (request: TypesGen.UpdateChatProjectInstructionsRequest) =>
			API.experimental.updateChatProjectInstructions(
				project.organization_id,
				project.id,
				request,
			),
		onSuccess: async (instructions) => {
			// A refetch that started before the write would otherwise
			// overwrite the saved value with the old one when it lands.
			await queryClient.cancelQueries({
				queryKey: chatProjectInstructionsKey(project.id),
			});
			queryClient.setQueryData(
				chatProjectInstructionsKey(project.id),
				instructions,
			);
		},
	});

export const deleteChatProjectInstructions = (
	queryClient: QueryClient,
	project: TypesGen.ChatProject,
) =>
	mutationOptions({
		mutationFn: () =>
			API.experimental.deleteChatProjectInstructions(
				project.organization_id,
				project.id,
			),
		onSuccess: async () => {
			await queryClient.cancelQueries({
				queryKey: chatProjectInstructionsKey(project.id),
			});
			queryClient.setQueryData<TypesGen.ChatProjectInstructions>(
				chatProjectInstructionsKey(project.id),
				{
					project_id: project.id,
					instructions: "",
					updated_by: null,
					updated_at: null,
				},
			);
		},
	});
