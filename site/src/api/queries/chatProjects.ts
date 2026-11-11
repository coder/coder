import { mutationOptions, type QueryClient, queryOptions } from "react-query";
import { API } from "#/api/api";
import { getErrorStatus } from "#/api/errors";
import type * as TypesGen from "#/api/typesGenerated";
import {
	chatEntitiesFamilyKey,
	invalidateChatListQueries,
	invalidateChatSearches,
	invalidateChatsByWorkspace,
} from "./chats";

export const chatProjectsKey = ["chat-projects"] as const;

/**
 * Lists chat projects the current user owns or that are shared with them,
 * across all organizations.
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

/**
 * Caches the archived-project list. It nests under chatProjectsKey, so
 * invalidating the active list also refetches it.
 */
export const archivedChatProjectsKey = [
	...chatProjectsKey,
	"archived",
] as const;

const unlistedChatProjectKey = (projectId: string | undefined) =>
	[...chatProjectsKey, projectId, "unlisted"] as const;

/**
 * Finds a project in any of the given organizations. Organization admins and
 * site Owners can manage projects that are neither theirs nor shared with
 * them, which the lists leave out. Data is null when no organization returns
 * the project.
 */
export const unlistedChatProject = (
	projectId: string | undefined,
	organizationIds: readonly string[],
) =>
	queryOptions({
		queryKey: [...unlistedChatProjectKey(projectId), organizationIds],
		queryFn: async () => {
			if (projectId === undefined) {
				return null;
			}
			// The server answers 404 for an organization that doesn't hold
			// the project or a project the user can't read.
			const projects = await Promise.all(
				organizationIds.map((organizationId) =>
					API.experimental
						.getChatProject(organizationId, projectId)
						.catch((error: unknown) => {
							if (getErrorStatus(error) === 404) {
								return null;
							}
							throw error;
						}),
				),
			);
			return projects.find((project) => project !== null) ?? null;
		},
	});

const archiveDialogKeySegment = "archive-dialog";

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

const isArchiveDialogKey = (queryKey: readonly unknown[]) =>
	queryKey[chatProjectsKey.length + 1] === archiveDialogKeySegment;

/** Refreshes the projects and every cached chat a project change can touch. */
const invalidateProjectsAndChats = (queryClient: QueryClient) =>
	Promise.all([
		queryClient.invalidateQueries({
			queryKey: chatProjectsKey,
			predicate: ({ queryKey }) => !isArchiveDialogKey(queryKey),
		}),
		// The archive dialog stays mounted while it closes after archiving, so
		// refetching its queries would flash the emptied project. The next
		// dialog fetches them fresh when it mounts.
		queryClient.invalidateQueries({
			queryKey: chatProjectsKey,
			predicate: ({ queryKey }) => isArchiveDialogKey(queryKey),
			refetchType: "none",
		}),
		invalidateChatListQueries(queryClient),
		invalidateChatSearches(queryClient),
		// Only the chats themselves carry project_id and archived. Their
		// nested queries (messages, ACL, diffs, ...) would refetch every
		// loaded page for nothing.
		queryClient.invalidateQueries({
			queryKey: chatEntitiesFamilyKey,
			predicate: ({ queryKey }) =>
				queryKey.length === chatEntitiesFamilyKey.length + 1,
		}),
	]);

export const deleteChatProject = (queryClient: QueryClient) =>
	mutationOptions({
		mutationFn: (project: TypesGen.ChatProject) =>
			API.experimental.deleteChatProject(project.organization_id, project.id),
		// Deleting a project clears project_id on its chats.
		onSettled: () => invalidateProjectsAndChats(queryClient),
	});

/**
 * Moves a project between the cached active and archived lists, so pages
 * that look it up keep finding it while both lists refetch. A project in
 * neither list stays out of them and is updated where unlistedChatProject
 * cached it.
 */
const setCachedProjectArchiveState = (
	queryClient: QueryClient,
	project: TypesGen.ChatProject,
) => {
	const [fromKey, toKey] = project.archived_at
		? [chatProjectsKey, archivedChatProjectsKey]
		: [archivedChatProjectsKey, chatProjectsKey];
	const isListed = queryClient
		.getQueryData<TypesGen.ChatProject[]>(fromKey)
		?.some(({ id }) => id === project.id);
	if (isListed) {
		queryClient.setQueryData<TypesGen.ChatProject[]>(fromKey, (projects) =>
			projects?.filter(({ id }) => id !== project.id),
		);
		queryClient.setQueryData<TypesGen.ChatProject[]>(toKey, (projects = []) => [
			project,
			...projects.filter(({ id }) => id !== project.id),
		]);
	}
	queryClient.setQueriesData<TypesGen.ChatProject | null>(
		{ queryKey: unlistedChatProjectKey(project.id) },
		(cached) => (cached ? project : cached),
	);
};

/** Refreshes what archiving or unarchiving a project changes. */
const invalidateProjectArchiveState = (queryClient: QueryClient) => {
	void invalidateProjectsAndChats(queryClient);
	// Archived chats no longer count as using their workspaces.
	void invalidateChatsByWorkspace(queryClient);
};

/**
 * Archives a project and every chat in it. Without stopActiveChats the
 * server refuses with 409 while any chat in the project is active.
 */
export const archiveChatProject = (queryClient: QueryClient) =>
	mutationOptions({
		mutationFn: ({
			project,
			stopActiveChats,
		}: {
			project: TypesGen.ChatProject;
			stopActiveChats: boolean;
		}) =>
			API.experimental.archiveChatProject(project.organization_id, project.id, {
				stop_active_chats: stopActiveChats,
			}),
		onSuccess: (project) => setCachedProjectArchiveState(queryClient, project),
		onSettled: () => invalidateProjectArchiveState(queryClient),
	});

/** Unarchives a project and the chats archived with it. */
export const unarchiveChatProject = (queryClient: QueryClient) =>
	mutationOptions({
		mutationFn: (project: TypesGen.ChatProject) =>
			API.experimental.unarchiveChatProject(
				project.organization_id,
				project.id,
			),
		onSuccess: (project) => setCachedProjectArchiveState(queryClient, project),
		onSettled: () => invalidateProjectArchiveState(queryClient),
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

export const chatProjectACLKey = (projectId: string) =>
	[...chatProjectsKey, projectId, "acl"] as const;

/** Lists a project's owner and the users and groups it is shared with. */
export const chatProjectACL = (project: TypesGen.ChatProject) =>
	queryOptions({
		queryKey: chatProjectACLKey(project.id),
		queryFn: () =>
			API.experimental.getChatProjectACL(project.organization_id, project.id),
	});

type SetChatProjectRoleVariables = {
	project: TypesGen.ChatProject;
	principal: { readonly kind: "user" | "group"; readonly id: string };
	role: TypesGen.ChatProjectRole;
};

/** Sets a user's or group's project role; an empty role removes access. */
export const setChatProjectRole = (queryClient: QueryClient) =>
	mutationOptions({
		mutationFn: ({ project, principal, role }: SetChatProjectRoleVariables) =>
			API.experimental.updateChatProjectACL(
				project.organization_id,
				project.id,
				{
					[principal.kind === "group" ? "group_roles" : "user_roles"]: {
						[principal.id]: role,
					},
				},
			),
		// Also refresh the project: a change or a 403 can revoke the viewer's admin.
		onSettled: (_data, _error, { project }) =>
			Promise.all([
				queryClient.invalidateQueries({
					queryKey: chatProjectsKey,
					exact: true,
				}),
				queryClient.invalidateQueries({
					queryKey: chatProjectACLKey(project.id),
					exact: true,
				}),
			]),
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
