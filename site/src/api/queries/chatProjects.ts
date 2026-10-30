import {
	mutationOptions,
	type QueryClient,
	type QueryKey,
	queryOptions,
} from "react-query";
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
			// Only the list changes. Keys under it hold the project's own data.
			queryClient.invalidateQueries({ queryKey: chatProjectsKey, exact: true }),
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
		onSettled: (_data, _error, { project, request }) =>
			Promise.all([
				// Only the list changes. Keys under it hold the project's own data.
				queryClient.invalidateQueries({
					queryKey: chatProjectsKey,
					exact: true,
				}),
				// A new MCP server may need the agent's token.
				request.mcp_server_ids &&
					queryClient.invalidateQueries({
						queryKey: chatProjectMCPServerTokensKey(project.id),
						exact: true,
					}),
			]),
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

export const chatProjectRunsKey = (projectId: string) =>
	[...chatProjectsKey, projectId, "runs"] as const;

/** Lists the newest chats that ran in an agent, including delegated runs. */
export const chatProjectRuns = (project: TypesGen.ChatProject) =>
	queryOptions({
		queryKey: chatProjectRunsKey(project.id),
		queryFn: () =>
			API.experimental.getChatProjectRuns(project.organization_id, project.id),
		refetchInterval: 15_000,
	});

export const chatProjectTriggersKey = (projectId: string) =>
	[...chatProjectsKey, projectId, "triggers"] as const;

/**
 * Lists an agent's triggers and polls them, so each one's last delivery
 * shows a GitHub ping or ignored event soon after it arrives. Polling pauses
 * while edits are saving, since a refetch would undo their optimistic changes.
 */
export const chatProjectTriggers = (
	queryClient: QueryClient,
	project: TypesGen.ChatProject,
) =>
	queryOptions({
		queryKey: chatProjectTriggersKey(project.id),
		queryFn: () =>
			API.experimental.getChatProjectTriggers(
				project.organization_id,
				project.id,
			),
		refetchInterval: () =>
			pendingTriggerUpdates(queryClient, project) > 0 ? false : 15_000,
	});

// A sibling of the triggers key, so the cancelQueries of a trigger edit
// does not cancel an open delivery list.
export const chatProjectTriggerDeliveriesKey = (
	projectId: string,
	triggerId: string,
) => [...chatProjectsKey, projectId, "trigger-deliveries", triggerId] as const;

/** The newest calls to a trigger's webhook and what each one did. */
export const chatProjectTriggerDeliveries = (
	project: TypesGen.ChatProject,
	triggerId: string,
) =>
	queryOptions({
		queryKey: chatProjectTriggerDeliveriesKey(project.id, triggerId),
		queryFn: () =>
			API.experimental.getChatProjectTriggerDeliveries(
				project.organization_id,
				project.id,
				triggerId,
			),
		refetchInterval: 15_000,
	});

const chatProjectTriggerUpdatesKey = (projectId: string) =>
	[...chatProjectTriggersKey(projectId), "update"] as const;

/** Counts the trigger edits of a project that are saving or queued. */
const pendingTriggerUpdates = (
	queryClient: QueryClient,
	project: TypesGen.ChatProject,
) =>
	queryClient.isMutating({
		mutationKey: chatProjectTriggerUpdatesKey(project.id),
	});

/** True while settling the only pending trigger update of a project. */
const isLastTriggerUpdate = (
	queryClient: QueryClient,
	project: TypesGen.ChatProject,
) => pendingTriggerUpdates(queryClient, project) <= 1;

export const createChatProjectTrigger = (
	queryClient: QueryClient,
	project: TypesGen.ChatProject,
) =>
	mutationOptions({
		// The response carries the webhook URL, so it does not linger.
		gcTime: 0,
		mutationFn: (request: TypesGen.CreateChatProjectTriggerRequest) =>
			API.experimental.createChatProjectTrigger(
				project.organization_id,
				project.id,
				request,
			),
		// With edits queued, the last edit's refetch picks up the new trigger;
		// refetching now would undo their optimistic changes.
		onSettled: async () => {
			if (pendingTriggerUpdates(queryClient, project) === 0) {
				await queryClient.invalidateQueries({
					queryKey: chatProjectTriggersKey(project.id),
				});
			}
		},
	});

export const updateChatProjectTrigger = (
	queryClient: QueryClient,
	project: TypesGen.ChatProject,
) =>
	mutationOptions({
		// The request may hold a new signing secret, so it does not linger.
		gcTime: 0,
		mutationFn: ({
			triggerId,
			request,
		}: {
			triggerId: string;
			request: TypesGen.UpdateChatProjectTriggerRequest;
		}) =>
			API.experimental.updateChatProjectTrigger(
				project.organization_id,
				project.id,
				triggerId,
				request,
			),
		// Edits save one at a time, in order, so quick changes cannot land
		// out of order.
		mutationKey: chatProjectTriggerUpdatesKey(project.id),
		scope: { id: `chat-project-trigger-updates-${project.id}` },
		// Applies the change to the cached list at once so inline edits feel
		// instant; onError puts the previous list back.
		onMutate: async ({ triggerId, request }) => {
			const queryKey = chatProjectTriggersKey(project.id);
			await queryClient.cancelQueries({ queryKey });
			const previous =
				queryClient.getQueryData<TypesGen.ChatProjectTrigger[]>(queryKey);
			// Secrets are write-only, so the cache only records whether one
			// is set.
			const { slack_signing_secret, github_webhook_secret, ...fields } =
				request;
			queryClient.setQueryData<TypesGen.ChatProjectTrigger[]>(
				queryKey,
				(triggers) =>
					triggers?.map((trigger) =>
						trigger.id === triggerId
							? {
									...trigger,
									...fields,
									...(slack_signing_secret !== undefined && {
										has_slack_signing_secret:
											slack_signing_secret.trim() !== "",
									}),
									...(github_webhook_secret !== undefined && {
										has_github_webhook_secret:
											github_webhook_secret.trim() !== "",
									}),
								}
							: trigger,
					),
			);
			return { previous };
		},
		onError: (_error, _variables, context) => {
			// With later edits still queued, the snapshot would also undo
			// them; the refetch after the last edit restores the server state.
			if (context?.previous && isLastTriggerUpdate(queryClient, project)) {
				queryClient.setQueryData(
					chatProjectTriggersKey(project.id),
					context.previous,
				);
			}
		},
		// Refetching while edits are queued would show the server state
		// without them, and the next edit would build on that stale list.
		onSettled: async () => {
			if (isLastTriggerUpdate(queryClient, project)) {
				await queryClient.invalidateQueries({
					queryKey: chatProjectTriggersKey(project.id),
				});
			}
		},
	});

export const deleteChatProjectTrigger = (
	queryClient: QueryClient,
	project: TypesGen.ChatProject,
) =>
	mutationOptions({
		mutationFn: (triggerId: string) =>
			API.experimental.deleteChatProjectTrigger(
				project.organization_id,
				project.id,
				triggerId,
			),
		onSettled: () =>
			Promise.all([
				// Queued edits refetch the list once the last one settles.
				pendingTriggerUpdates(queryClient, project) === 0 &&
					queryClient.invalidateQueries({
						queryKey: chatProjectTriggersKey(project.id),
					}),
				queryClient.invalidateQueries({
					queryKey: chatProjectRunsKey(project.id),
				}),
			]),
	});

/** Replaces a trigger's secret and returns its new webhook URL, shown once. */
export const regenerateChatProjectTriggerWebhookURL = (
	project: TypesGen.ChatProject,
) =>
	mutationOptions({
		// The response carries the webhook URL, so it does not linger.
		gcTime: 0,
		mutationFn: (triggerId: string) =>
			API.experimental.regenerateChatProjectTriggerWebhookURL(
				project.organization_id,
				project.id,
				triggerId,
			),
	});

/** Runs a schedule trigger now and refreshes its last run and the agent's chats. */
export const runChatProjectTrigger = (
	queryClient: QueryClient,
	project: TypesGen.ChatProject,
) =>
	mutationOptions({
		mutationFn: (triggerId: string) =>
			API.experimental.runChatProjectTrigger(
				project.organization_id,
				project.id,
				triggerId,
			),
		// A failed run is recorded as a delivery too.
		onSettled: (_data, _error, triggerId) =>
			Promise.all([
				// Queued edits refetch the list once the last one settles.
				pendingTriggerUpdates(queryClient, project) === 0 &&
					queryClient.invalidateQueries({
						queryKey: chatProjectTriggersKey(project.id),
					}),
				queryClient.invalidateQueries({
					queryKey: chatProjectTriggerDeliveriesKey(project.id, triggerId),
				}),
				queryClient.invalidateQueries({
					queryKey: chatProjectRunsKey(project.id),
				}),
			]),
	});

export const chatProjectACLKey = (projectId: string) =>
	[...chatProjectsKey, projectId, "acl"] as const;

export const chatProjectACL = (project: TypesGen.ChatProject) =>
	queryOptions({
		queryKey: chatProjectACLKey(project.id),
		queryFn: () =>
			API.experimental.getChatProjectACL(project.organization_id, project.id),
	});

export const chatProjectACLAvailable = (
	project: TypesGen.ChatProject,
	options: TypesGen.UsersRequest,
) =>
	queryOptions({
		// The principal picker types its options with the loose key type.
		queryKey: [
			...chatProjectACLKey(project.id),
			"available",
			options,
		] as QueryKey,
		queryFn: () =>
			API.experimental.getChatProjectACLAvailable(
				project.organization_id,
				project.id,
				options,
			),
	});

export const updateChatProjectACL = (
	queryClient: QueryClient,
	project: TypesGen.ChatProject,
) =>
	mutationOptions({
		mutationFn: (request: TypesGen.UpdateChatProjectACLRequest) =>
			API.experimental.updateChatProjectACL(
				project.organization_id,
				project.id,
				request,
			),
		onSettled: () =>
			queryClient.invalidateQueries({
				queryKey: chatProjectACLKey(project.id),
				exact: true,
			}),
	});

export const chatProjectExternalAuthKey = (projectId: string) =>
	[...chatProjectsKey, projectId, "external-auth"] as const;

/** The external auth providers and links of the agent's user. */
export const chatProjectExternalAuth = (project: TypesGen.ChatProject) =>
	queryOptions({
		queryKey: chatProjectExternalAuthKey(project.id),
		queryFn: () =>
			API.experimental.getChatProjectExternalAuth(
				project.organization_id,
				project.id,
			),
	});

export const connectChatProjectExternalAuth = (project: TypesGen.ChatProject) =>
	mutationOptions({
		mutationFn: ({
			providerId,
			redirect,
		}: {
			providerId: string;
			redirect: string;
		}) =>
			API.experimental.connectChatProjectExternalAuth(
				project.organization_id,
				project.id,
				providerId,
				{ redirect },
			),
	});

/** Exchanges a device code. Fails with a retryable error while pending. */
export const exchangeChatProjectExternalAuthDevice = (
	project: TypesGen.ChatProject,
	providerId: string,
	deviceCode: string,
) =>
	queryOptions({
		queryKey: [
			...chatProjectExternalAuthKey(project.id),
			providerId,
			"device",
			deviceCode,
		] as const,
		queryFn: () =>
			API.experimental.exchangeChatProjectExternalAuthDevice(
				project.organization_id,
				project.id,
				providerId,
				{ device_code: deviceCode },
			),
	});

/**
 * The one-time device code for connecting an account for the agent. It has its
 * own key and is never cached, because a code works once and the external auth
 * page keeps its own under the default key.
 */
export const chatProjectExternalAuthDeviceCode = (
	project: TypesGen.ChatProject,
	providerId: string,
) =>
	queryOptions({
		queryKey: [
			...chatProjectExternalAuthKey(project.id),
			providerId,
			"device-code",
		] as const,
		queryFn: () => API.getExternalAuthDevice(providerId),
		gcTime: 0,
	});

export const deleteChatProjectExternalAuth = (
	queryClient: QueryClient,
	project: TypesGen.ChatProject,
) =>
	mutationOptions({
		mutationFn: (providerId: string) =>
			API.experimental.deleteChatProjectExternalAuth(
				project.organization_id,
				project.id,
				providerId,
			),
		onSettled: () =>
			queryClient.invalidateQueries({
				queryKey: chatProjectExternalAuthKey(project.id),
			}),
	});

export const chatProjectMCPServerTokensKey = (projectId: string) =>
	[...chatProjectsKey, projectId, "mcp-server-tokens"] as const;

/** The agent's MCP servers and whether its user has a token for each. */
export const chatProjectMCPServerTokens = (project: TypesGen.ChatProject) =>
	queryOptions({
		queryKey: chatProjectMCPServerTokensKey(project.id),
		queryFn: () =>
			API.experimental.getChatProjectMCPServerTokens(
				project.organization_id,
				project.id,
			),
	});

export const updateChatProjectMCPServerToken = (
	queryClient: QueryClient,
	project: TypesGen.ChatProject,
) =>
	mutationOptions({
		// The variables hold the token or private key, so they do not linger.
		gcTime: 0,
		mutationFn: ({
			serverId,
			request,
		}: {
			serverId: string;
			request: TypesGen.UpdateMCPServerUserTokenRequest;
		}) =>
			API.experimental.updateChatProjectMCPServerToken(
				project.organization_id,
				project.id,
				serverId,
				request,
			),
		onSettled: () =>
			queryClient.invalidateQueries({
				queryKey: chatProjectMCPServerTokensKey(project.id),
				exact: true,
			}),
	});

export const deleteChatProjectMCPServerToken = (
	queryClient: QueryClient,
	project: TypesGen.ChatProject,
) =>
	mutationOptions({
		mutationFn: (serverId: string) =>
			API.experimental.deleteChatProjectMCPServerToken(
				project.organization_id,
				project.id,
				serverId,
			),
		onSettled: () =>
			queryClient.invalidateQueries({
				queryKey: chatProjectMCPServerTokensKey(project.id),
				exact: true,
			}),
	});
