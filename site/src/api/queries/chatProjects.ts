import { mutationOptions, type QueryClient, queryOptions } from "react-query";
import { API } from "#/api/api";
import type * as TypesGen from "#/api/typesGenerated";
import {
	chatEntitiesFamilyKey,
	chatListFamilyKey,
	invalidateChatListQueries,
	invalidateChatSearches,
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
