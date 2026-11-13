import { mutationOptions, type QueryClient, queryOptions } from "react-query";
import { API } from "#/api/api";
import { getErrorStatus } from "#/api/errors";
import type * as TypesGen from "#/api/typesGenerated";
import {
	chatEntitiesFamilyKey,
	chatListFamilyKey,
	DEFAULT_CHAT_PAGE_LIMIT,
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

/** Lists the archived projects the current user owns or that are shared with them. */
export const archivedChatProjects = () =>
	queryOptions({
		queryKey: archivedChatProjectsKey,
		queryFn: () => API.experimental.getChatProjects({ archived: true }),
	});

/**
 * Reads one project from the archivedChatProjects() list. Data is null when
 * the project is not archived or not visible to the user.
 */
export const archivedChatProject = (projectId: string | undefined) =>
	queryOptions({
		...archivedChatProjects(),
		select: (projects) =>
			projects.find((project) => project.id === projectId) ?? null,
	});

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

/** Prefixes the queries the archive dialog reads for one project. */
export const chatProjectArchiveDialogKey = (projectId: string) =>
	[...chatProjectsKey, projectId, archiveDialogKeySegment] as const;

export const chatProjectArchivePreviewKey = (projectId: string) =>
	[...chatProjectArchiveDialogKey(projectId), "preview"] as const;

/**
 * Caches the workspaces behind the user's own unarchived chats in a project,
 * under the archive dialog's queries so they refresh together.
 */
export const projectChatWorkspacesKey = (projectId: string) =>
	[...chatProjectArchiveDialogKey(projectId), "own-chat-workspaces"] as const;

/** Reads every page of the user's own unarchived chats in a project. */
export const listOwnProjectChats = async (
	projectId: string,
	signal: AbortSignal,
) => {
	const chats: TypesGen.Chat[] = [];
	for (let offset = 0; ; offset += DEFAULT_CHAT_PAGE_LIMIT) {
		const page = await API.experimental.getChats(
			{
				limit: DEFAULT_CHAT_PAGE_LIMIT,
				offset,
				q: "archived:false source:created_by_me",
				project_id: projectId,
			},
			signal,
		);
		chats.push(...page);
		if (page.length < DEFAULT_CHAT_PAGE_LIMIT) {
			return chats;
		}
	}
};

/** Counts the chats that archiving a project archives and stops. */
export const chatProjectArchivePreview = (project: TypesGen.ChatProject) =>
	queryOptions({
		queryKey: chatProjectArchivePreviewKey(project.id),
		queryFn: () =>
			API.experimental.getChatProjectArchivePreview(
				project.organization_id,
				project.id,
			),
	});

/** Nested under chatProjectsKey, so project list invalidations reach it. */
export const chatProjectSummariesKey = [
	...chatProjectsKey,
	"summaries",
] as const;

/**
 * The user's projects with their pins and counts over the user's own chats.
 * Pins send no watch events, so other tabs catch up on focus.
 */
export const chatProjectSummaries = () =>
	queryOptions({
		queryKey: chatProjectSummariesKey,
		queryFn: () => API.experimental.getChatProjectSummaries(),
		refetchOnWindowFocus: true,
	});

/** The pinned summaries, in pin order. */
export const getPinnedProjectSummaries = (
	summaries: readonly TypesGen.ChatProjectSummary[],
): TypesGen.ChatProjectSummary[] =>
	summaries
		.filter((summary) => summary.pinned)
		.sort((a, b) => a.pin_order - b.pin_order);

const projectPinsMutationKey = ["chat-project-pins"] as const;

// Pin changes share a scope, so the server applies them in the order the
// user made them.
const projectPinsMutationOptions = {
	mutationKey: projectPinsMutationKey,
	scope: { id: "chat-project-pins" },
} as const;

// An earlier queued pin change would replace the later changes' optimistic
// order with the server's, so only the last one restores or refreshes it.
const isLastProjectPinChange = (queryClient: QueryClient) =>
	queryClient.isMutating({ mutationKey: projectPinsMutationKey }) <= 1;

// The sidebar's chat list leaves out chats in unpinned projects, so pinning
// or unpinning can add or remove chats from it; moving a pin cannot. Project
// chat lists do not change.
const invalidateProjectPins = (
	queryClient: QueryClient,
	{ changesChatList }: { changesChatList: boolean },
) =>
	Promise.all([
		isLastProjectPinChange(queryClient) &&
			queryClient.invalidateQueries({ queryKey: chatProjectSummariesKey }),
		changesChatList &&
			queryClient.invalidateQueries({ queryKey: chatListFamilyKey }),
	]);

type PinChatProjectVariables = {
	project: TypesGen.ChatProject;
	pinOrder?: number;
};

/** Pins a project at the end, or moves a pinned project to pinOrder. */
export const pinChatProject = (queryClient: QueryClient) =>
	mutationOptions({
		...projectPinsMutationOptions,
		mutationFn: ({ project, pinOrder }: PinChatProjectVariables) =>
			API.experimental.pinChatProject(
				project.organization_id,
				project.id,
				pinOrder,
			),
		// A move shows at once, and a summaries fetch already in flight cannot
		// put the project back while the request runs.
		onMutate: async ({ project, pinOrder }: PinChatProjectVariables) => {
			if (pinOrder === undefined) {
				return { previousSummaries: undefined, isMove: false };
			}
			await queryClient.cancelQueries({ queryKey: chatProjectSummariesKey });
			const previousSummaries = queryClient.getQueryData<
				TypesGen.ChatProjectSummary[]
			>(chatProjectSummariesKey);
			const previousPinnedIds = getPinnedProjectSummaries(
				previousSummaries ?? [],
			).map((s) => s.project.id);
			const pinnedIds = previousPinnedIds
				.filter((id) => id !== project.id)
				.toSpliced(pinOrder - 1, 0, project.id);
			const pinOrderById = new Map(pinnedIds.map((id, i) => [id, i + 1]));
			queryClient.setQueryData<TypesGen.ChatProjectSummary[]>(
				chatProjectSummariesKey,
				(summaries) =>
					summaries?.map((s) => {
						const order = pinOrderById.get(s.project.id);
						return order === undefined
							? s
							: { ...s, pinned: true, pin_order: order };
					}),
			);
			return {
				previousSummaries,
				isMove: previousPinnedIds.includes(project.id),
			};
		},
		onError: (_error, _variables, context) => {
			if (context?.previousSummaries && isLastProjectPinChange(queryClient)) {
				queryClient.setQueryData(
					chatProjectSummariesKey,
					context.previousSummaries,
				);
			}
		},
		onSettled: (_data, _error, _variables, context) =>
			invalidateProjectPins(queryClient, {
				changesChatList: !context?.isMove,
			}),
	});

/** Unpins a project, which also takes its chats out of the sidebar. */
export const unpinChatProject = (queryClient: QueryClient) =>
	mutationOptions({
		...projectPinsMutationOptions,
		mutationFn: (project: TypesGen.ChatProject) =>
			API.experimental.unpinChatProject(project.organization_id, project.id),
		// The folder and its chats leave the sidebar at once, and a summaries
		// fetch already in flight cannot bring them back.
		onMutate: async (project: TypesGen.ChatProject) => {
			await queryClient.cancelQueries({ queryKey: chatProjectSummariesKey });
			const previousSummaries = queryClient.getQueryData<
				TypesGen.ChatProjectSummary[]
			>(chatProjectSummariesKey);
			queryClient.setQueryData<TypesGen.ChatProjectSummary[]>(
				chatProjectSummariesKey,
				(summaries) =>
					summaries?.map((s) =>
						s.project.id === project.id
							? { ...s, pinned: false, pin_order: 0 }
							: s,
					),
			);
			return { previousSummaries };
		},
		onError: (_error, _project, context) => {
			if (context?.previousSummaries && isLastProjectPinChange(queryClient)) {
				queryClient.setQueryData(
					chatProjectSummariesKey,
					context.previousSummaries,
				);
			}
		},
		onSettled: () =>
			invalidateProjectPins(queryClient, { changesChatList: true }),
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

	// Summaries list only active projects. An unarchived project returns
	// when they refetch.
	if (project.archived_at) {
		queryClient.setQueryData<TypesGen.ChatProjectSummary[]>(
			chatProjectSummariesKey,
			(summaries) => summaries?.filter((s) => s.project.id !== project.id),
		);
	}
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
				// Summaries carry the project's permissions and member count.
				queryClient.invalidateQueries({ queryKey: chatProjectSummariesKey }),
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
