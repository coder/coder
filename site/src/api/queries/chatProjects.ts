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
 * Lists chat projects the current user owns or that are shared with them,
 * across all organizations. Summaries name their projects by ID and refetch
 * on focus, so the list does too: a project created or shared since the
 * list loaded would otherwise have a summary without a project.
 * @public
 */
export const chatProjects = () =>
	queryOptions({
		queryKey: chatProjectsKey,
		queryFn: () => API.experimental.getChatProjects(),
		refetchOnWindowFocus: true,
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

/** Nested under chatProjectsKey, so project list invalidations reach it. */
export const chatProjectSummariesKey = [
	...chatProjectsKey,
	"summaries",
] as const;

/**
 * The user's pin and counts over the user's own chats for each project in
 * chatProjects(), by project ID. Pins send no watch events, so other tabs
 * catch up on focus.
 */
export const chatProjectSummaries = () =>
	queryOptions({
		queryKey: chatProjectSummariesKey,
		queryFn: () => API.experimental.getChatProjectSummaries(),
		refetchOnWindowFocus: true,
	});

/** Nested under chatProjectsKey, so project list invalidations reach it. */
export const chatProjectMemberCountsKey = [
	...chatProjectsKey,
	"member-counts",
] as const;

/** How many users can use each project in chatProjects(), by project ID. */
export const chatProjectMemberCounts = () =>
	queryOptions({
		queryKey: chatProjectMemberCountsKey,
		queryFn: () => API.experimental.getChatProjectMemberCounts(),
	});

/** A summary with its project from chatProjects(). */
export type ChatProjectWithSummary = TypesGen.ChatProjectSummary & {
	readonly project: TypesGen.ChatProject;
};

/**
 * Pairs each summary with its project, in the summaries' order. A summary
 * whose project the list lacks is left out until the list refetches.
 */
export const joinChatProjectSummaries = (
	projects: readonly TypesGen.ChatProject[],
	summaries: readonly TypesGen.ChatProjectSummary[],
): ChatProjectWithSummary[] => {
	const projectsById = new Map(
		projects.map((project) => [project.id, project]),
	);
	return summaries.flatMap((summary) => {
		const project = projectsById.get(summary.project_id);
		return project ? [{ ...summary, project }] : [];
	});
};

/** The pinned summaries, in pin order. */
export const getPinnedProjectSummaries = <
	T extends TypesGen.ChatProjectSummary,
>(
	summaries: readonly T[],
): T[] =>
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

const invalidateProjectPins = async (queryClient: QueryClient) => {
	if (isLastProjectPinChange(queryClient)) {
		await queryClient.invalidateQueries({ queryKey: chatProjectSummariesKey });
	}
};

/**
 * Pins a project after the projects that are already pinned. The variables
 * wrap the project, so an observer of the shared mutation key can tell a pin
 * from the other pin changes.
 */
export const pinChatProject = (queryClient: QueryClient) =>
	mutationOptions({
		...projectPinsMutationOptions,
		mutationFn: ({ project }: { project: TypesGen.ChatProject }) =>
			API.experimental.pinChatProject(project.organization_id, project.id),
		onSettled: () => invalidateProjectPins(queryClient),
	});

/**
 * Saves the order of the pinned projects. As on the server, pins left out of
 * projectIds follow the listed ones in their current order.
 */
export const reorderChatProjectPins = (queryClient: QueryClient) =>
	mutationOptions({
		...projectPinsMutationOptions,
		mutationFn: (projectIds: readonly string[]) =>
			API.experimental.reorderChatProjectPins({ project_ids: projectIds }),
		// The new order shows at once, and a summaries fetch already in flight
		// cannot put the projects back while the request runs.
		onMutate: async (projectIds: readonly string[]) => {
			await queryClient.cancelQueries({ queryKey: chatProjectSummariesKey });
			const previousSummaries = queryClient.getQueryData<
				TypesGen.ChatProjectSummary[]
			>(chatProjectSummariesKey);
			const listedAt = new Map(projectIds.map((id, index) => [id, index]));
			const place = (summary: TypesGen.ChatProjectSummary) =>
				listedAt.get(summary.project_id) ?? projectIds.length;
			const pinOrderById = new Map(
				getPinnedProjectSummaries(previousSummaries ?? [])
					.sort((a, b) => place(a) - place(b))
					.map((summary, index) => [summary.project_id, index + 1]),
			);
			queryClient.setQueryData<TypesGen.ChatProjectSummary[]>(
				chatProjectSummariesKey,
				(summaries) =>
					summaries?.map((s) => {
						const order = pinOrderById.get(s.project_id);
						return order === undefined ? s : { ...s, pin_order: order };
					}),
			);
			return { previousSummaries };
		},
		onError: (_error, _projectIds, context) => {
			if (context?.previousSummaries && isLastProjectPinChange(queryClient)) {
				queryClient.setQueryData(
					chatProjectSummariesKey,
					context.previousSummaries,
				);
			}
		},
		onSettled: () => invalidateProjectPins(queryClient),
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
						s.project_id === project.id
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
		onSettled: () => invalidateProjectPins(queryClient),
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
			// Deleting a project clears project_id on its chats.
			Promise.all([
				queryClient.invalidateQueries({ queryKey: chatProjectsKey }),
				invalidateChatListQueries(queryClient),
				invalidateChatSearches(queryClient),
				// Only the chats themselves carry project_id. Their nested
				// queries (messages, ACL, diffs, ...) would refetch every loaded
				// page for nothing.
				queryClient.invalidateQueries({
					queryKey: chatEntitiesFamilyKey,
					predicate: ({ queryKey }) =>
						queryKey.length === chatEntitiesFamilyKey.length + 1,
				}),
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
				queryClient.invalidateQueries({
					queryKey: chatProjectMemberCountsKey,
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
