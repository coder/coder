import { mutationOptions, type QueryClient, queryOptions } from "react-query";
import { API } from "#/api/api";
import { getErrorMessage, isWorkspaceNotFound } from "#/api/errors";
import { chatProjectArchiveDialogKey } from "#/api/queries/chatProjects";
import {
	ArchiveAndDeleteError,
	chatsByWorkspace,
	DEFAULT_CHAT_PAGE_LIMIT,
	invalidateChatsByWorkspace,
} from "#/api/queries/chats";
import { workspaceBuildsKey } from "#/api/queries/workspaceBuilds";
import { workspaceQuotaKey } from "#/api/queries/workspaceQuota";
import {
	invalidateWorkspaceListQueries,
	workspaceById,
	workspaceByIdKey,
} from "#/api/queries/workspaces";
import {
	type Chat,
	type ChatProject,
	PrebuildsSystemUserID,
	type Workspace,
	type WorkspaceBuild,
} from "#/api/typesGenerated";
import { DEFAULT_RECORDS_PER_PAGE } from "#/components/PaginationWidget/utils";
import { toast } from "#/components/Toaster/toast";
import { isWorkspaceOn } from "#/utils/workspace";

/**
 * Returns the moment a workspace's identity transferred to its
 * current owner.
 *
 * For workspaces created from scratch, this is `workspace.created_at`:
 * build #1 already belongs to the current owner.
 *
 * For workspaces claimed from a prebuild, this is the start time of
 * build #2 (the claim build). `workspace.created_at` for those
 * workspaces reflects when the prebuild was provisioned, often well
 * before the chat that claimed it existed, which is why the original
 * `created_at` heuristic misfired the deletion confirmation dialog.
 *
 * Returns `null` when the result cannot be determined, for example
 * an unclaimed prebuild (build #1 by prebuilds system user, no build
 * #2). Callers should treat `null` as "force the confirmation
 * dialog"; the deletion path is destructive and should err on the
 * side of asking.
 */
export function workspaceAcquiredAt(
	workspace: { created_at: string },
	builds: readonly Pick<
		WorkspaceBuild,
		"build_number" | "initiator_id" | "created_at"
	>[],
): string | null {
	const build1 = builds.find((b) => b.build_number === 1);
	// No history at all (shouldn't happen for an existing workspace);
	// fall back to created_at rather than blocking on missing data.
	if (!build1) {
		return workspace.created_at;
	}
	if (build1.initiator_id !== PrebuildsSystemUserID) {
		return workspace.created_at;
	}
	const build2 = builds.find((b) => b.build_number === 2);
	return build2 ? build2.created_at : null;
}

/**
 * Determines whether a workspace was auto-created by a chat. A
 * workspace is "auto-created" if the chat acquired it (via creation
 * from scratch or by claiming a prebuild) at or after the chat's own
 * creation time.
 *
 * Pre-existing workspaces that were manually associated with the
 * chat need a confirmation dialog before deletion.
 */
export function isWorkspaceAutoCreated(
	workspace: { created_at: string },
	builds: readonly Pick<
		WorkspaceBuild,
		"build_number" | "initiator_id" | "created_at"
	>[],
	chatCreatedAt: string,
): boolean {
	const acquiredAt = workspaceAcquiredAt(workspace, builds);
	if (acquiredAt === null) {
		return false;
	}
	return new Date(acquiredAt) >= new Date(chatCreatedAt);
}

/**
 * Returns whether the browser should navigate to /agents after an
 * archive-and-delete mutation settles. Navigation is appropriate
 * when the user is still viewing the archived chat or one of its
 * sub-agents; if they already navigated elsewhere the redirect
 * would be disruptive.
 */
export function shouldNavigateAfterArchive(
	activeChatId: string | undefined,
	archivedChatId: string,
	activeRootChatId?: string,
): boolean {
	if (activeChatId === archivedChatId) return true;
	// The active chat is a sub-agent rooted at the archived parent.
	if (activeRootChatId != null && activeRootChatId === archivedChatId) {
		return true;
	}
	return false;
}

/**
 * Resolves whether an archive-and-delete action should proceed
 * immediately or require user confirmation. Fetches the workspace
 * and its build history to determine when the workspace was
 * acquired (claim time for prebuilts, creation time otherwise) and
 * compares against the chat's creation time. Auto-created
 * workspaces (provisioned or claimed by the chat) skip the
 * confirmation dialog; pre-existing workspaces require the user to
 * type the workspace name.
 *
 * @param fetchWorkspace - Retrieves the workspace (e.g. via
 *   `queryClient.fetchQuery`). The result must include `created_at`.
 * @param fetchBuilds - Retrieves the workspace's build history. The
 *   first call only needs build_number 1 and 2, but callers will
 *   typically pass the full list.
 * @param getChatCreatedAt - Returns the chat's `created_at`
 *   timestamp, or `undefined` if the chat is not in the cache.
 * @returns `"proceed"` to skip the dialog, `"archive-only"` to archive
 *   without deleting because the workspace is already gone, or
 *   `"confirm"` to show the dialog.
 */
export type ArchiveAndDeleteAction = "proceed" | "confirm" | "archive-only";

export async function resolveArchiveAndDeleteAction(
	fetchWorkspace: () => Promise<{ created_at: string }>,
	fetchBuilds: () => Promise<
		readonly Pick<
			WorkspaceBuild,
			"build_number" | "initiator_id" | "created_at"
		>[]
	>,
	getChatCreatedAt: () => string | undefined,
): Promise<ArchiveAndDeleteAction> {
	let workspace: { created_at: string };
	try {
		workspace = await fetchWorkspace();
	} catch (error) {
		if (isWorkspaceNotFound(error)) {
			return "archive-only";
		}
		throw error;
	}
	const chatCreatedAt = getChatCreatedAt();
	if (!chatCreatedAt) {
		return "confirm";
	}
	let builds: readonly Pick<
		WorkspaceBuild,
		"build_number" | "initiator_id" | "created_at"
	>[];
	try {
		builds = await fetchBuilds();
	} catch (error) {
		if (isWorkspaceNotFound(error)) {
			return "archive-only";
		}
		throw error;
	}
	if (isWorkspaceAutoCreated(workspace, builds, chatCreatedAt)) {
		return "proceed";
	}
	return "confirm";
}

/**
 * Resolves the archive-and-delete action for a chat's workspace through
 * the query cache, so the workspace is cached for the confirmation
 * dialog and the failure toast by the time the action is known.
 */
export function fetchArchiveAndDeleteAction(
	queryClient: QueryClient,
	workspaceId: string,
	chatCreatedAt: string | undefined,
) {
	return resolveArchiveAndDeleteAction(
		() => queryClient.fetchQuery(workspaceById(workspaceId)),
		// Only builds 1 and 2 matter for recognising a prebuild claim. The
		// default page is newest-first; the resolver degrades safely
		// ("confirm") if those builds are not in the returned slice.
		() =>
			queryClient.fetchQuery({
				queryKey: [
					...workspaceBuildsKey(workspaceId),
					"archive-and-delete-resolver",
				],
				queryFn: () => API.getWorkspaceBuilds(workspaceId),
			}),
		() => chatCreatedAt,
	);
}

export function notifyDeleteQueueState(
	workspace: Workspace | undefined,
	deleteBuild: WorkspaceBuild | null,
): void {
	if (!deleteBuild || !workspace) {
		return;
	}
	const matched = deleteBuild.matched_provisioners;
	if (matched && matched.count === 0) {
		toast.warning(
			`Delete enqueued for "${workspace.name}", but no matching provisioners are available. The workspace will be deleted once one comes online.`,
		);
	}
}

export function notifyArchiveAndDeleteFailed(
	workspace: Workspace | undefined,
	error: unknown,
	onOpenWorkspace: (path: string) => void,
): void {
	const step = error instanceof ArchiveAndDeleteError ? error.step : undefined;
	const cause = error instanceof ArchiveAndDeleteError ? error.cause : error;

	if (step === "archive") {
		const label = workspace ? `"${workspace.name}"` : "the workspace";
		const prefix = `Failed to archive the chat for ${label}.`;
		const detail = getErrorMessage(cause, "");
		toast.error(detail ? `${prefix} ${detail}` : prefix);
		return;
	}

	const description =
		"The chat was archived but the workspace delete failed. Unarchive it from the archived filter, or open the workspace to delete it manually.";

	if (!workspace) {
		toast.error(getErrorMessage(cause, "Failed to delete workspace."), {
			description,
		});
		return;
	}

	const path = `/@${workspace.owner_name}/${workspace.name}`;
	toast.error(
		getErrorMessage(cause, `Failed to delete workspace "${workspace.name}".`),
		{
			description,
			action: {
				label: "Open workspace",
				onClick: () => onOpenWorkspace(path),
			},
		},
	);
}

/** A workspace behind one of the user's own chats in a project. */
export type ProjectChatWorkspace = {
	readonly workspace: Workspace;
	/** True when the workspace existed before the chats that use it. */
	readonly existedBeforeChat: boolean;
};

/** "leave" is a UI choice; only stop and delete reach the workspace mutation. */
export type ProjectWorkspaceAction = "leave" | "stop" | "delete";

const listOwnProjectChats = async (projectId: string, signal: AbortSignal) => {
	const chats: Chat[] = [];
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

const workspaceClassificationBatchSize = 5;

export const projectChatWorkspacesKey = (projectId: string) =>
	[...chatProjectArchiveDialogKey(projectId), "own-chat-workspaces"] as const;

/**
 * Lists the workspaces behind the user's own unarchived chats in a project,
 * by name. Workspaces that no longer exist are left out.
 */
export const projectChatWorkspaces = (
	queryClient: QueryClient,
	project: ChatProject,
) =>
	queryOptions({
		queryKey: projectChatWorkspacesKey(project.id),
		queryFn: async ({ signal }): Promise<ProjectChatWorkspace[]> => {
			const chats = await listOwnProjectChats(project.id, signal);
			// A workspace counts as created by a chat when it appeared after the
			// first chat that uses it.
			const firstChatCreatedAt = new Map<string, string>();
			for (const chat of chats.flatMap((root) => [
				root,
				...(root.children ?? []),
			])) {
				if (!chat.workspace_id) {
					continue;
				}
				const current = firstChatCreatedAt.get(chat.workspace_id);
				if (!current || new Date(chat.created_at) < new Date(current)) {
					firstChatCreatedAt.set(chat.workspace_id, chat.created_at);
				}
			}
			const classify = async ([workspaceId, chatCreatedAt]: [
				string,
				string,
			]): Promise<ProjectChatWorkspace[]> => {
				const action = await fetchArchiveAndDeleteAction(
					queryClient,
					workspaceId,
					chatCreatedAt,
				);
				const workspace = queryClient.getQueryData<Workspace>(
					workspaceByIdKey(workspaceId),
				);
				if (action === "archive-only" || !workspace) {
					return [];
				}
				return [{ workspace, existedBeforeChat: action === "confirm" }];
			};
			// Each workspace costs a workspace and a build history request, so
			// a project with many workspaces fetches them a batch at a time.
			const entries = [...firstChatCreatedAt];
			const results: ProjectChatWorkspace[] = [];
			for (
				let i = 0;
				i < entries.length;
				i += workspaceClassificationBatchSize
			) {
				const batch = entries.slice(i, i + workspaceClassificationBatchSize);
				results.push(...(await Promise.all(batch.map(classify))).flat());
			}
			return results.sort((a, b) =>
				a.workspace.name.localeCompare(b.workspace.name),
			);
		},
	});

// GET /chats/by-workspace accepts at most one page of workspace IDs.
const findWorkspacesInUse = async (
	queryClient: QueryClient,
	workspaceIds: readonly string[],
) => {
	const inUse = new Set<string>();
	for (let i = 0; i < workspaceIds.length; i += DEFAULT_RECORDS_PER_PAGE) {
		const chatByWorkspace = await queryClient.fetchQuery({
			...chatsByWorkspace(workspaceIds.slice(i, i + DEFAULT_RECORDS_PER_PAGE)),
			staleTime: 0,
		});
		for (const workspaceId of Object.keys(chatByWorkspace)) {
			inUse.add(workspaceId);
		}
	}
	return inUse;
};

export type ProjectWorkspaceActionResult = {
	readonly failed: ReadonlyArray<{
		readonly workspace: Workspace;
		readonly error: unknown;
	}>;
	/** Workspaces another unarchived chat still uses. */
	readonly skipped: readonly Workspace[];
};

/**
 * Stops or deletes the workspaces behind the user's chats once their project
 * is archived. Workspaces another unarchived chat still uses are skipped, and
 * only workspaces running at that moment are stopped. A failed in-use check rejects
 * before any workspace changes.
 */
export const projectWorkspaceAction = (queryClient: QueryClient) =>
	mutationOptions({
		mutationFn: async ({
			action,
			workspaces,
		}: {
			action: Exclude<ProjectWorkspaceAction, "leave">;
			workspaces: readonly Workspace[];
			/** The caller's toast for the outcome; the requests ignore it. */
			toastId?: string | number;
		}): Promise<ProjectWorkspaceActionResult> => {
			const inUse = await findWorkspacesInUse(
				queryClient,
				workspaces.map(({ id }) => id),
			);
			const failed: Array<{ workspace: Workspace; error: unknown }> = [];
			await Promise.all(
				workspaces
					.filter(({ id }) => !inUse.has(id))
					.map(async (workspace) => {
						try {
							if (action === "delete") {
								notifyDeleteQueueState(
									workspace,
									await API.deleteWorkspace(workspace.id),
								);
							} else {
								// The dialog's copy can be stale: the workspace may have
								// started or stopped while the dialog was open.
								const current = await queryClient.fetchQuery({
									...workspaceById(workspace.id),
									staleTime: 0,
								});
								if (isWorkspaceOn(current)) {
									await API.stopWorkspace(workspace.id);
								}
							}
						} catch (error) {
							if (!isWorkspaceNotFound(error)) {
								failed.push({ workspace, error });
							}
						}
					}),
			);
			return {
				failed,
				skipped: workspaces.filter(({ id }) => inUse.has(id)),
			};
		},
		onSettled: (_result, _error, { action, workspaces }) => {
			void invalidateWorkspaceListQueries(queryClient);
			void invalidateChatsByWorkspace(queryClient);
			for (const workspace of workspaces) {
				void queryClient.invalidateQueries({
					queryKey: workspaceByIdKey(workspace.id),
				});
			}
			if (action === "delete") {
				void queryClient.invalidateQueries({ queryKey: workspaceQuotaKey });
			}
		},
	});
