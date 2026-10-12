import type { QueryClient } from "react-query";
import { toast } from "sonner";
import { API } from "#/api/api";
import { getErrorMessage, isWorkspaceNotFound } from "#/api/errors";
import { ArchiveAndDeleteError } from "#/api/queries/chats";
import { workspaceBuildsKey } from "#/api/queries/workspaceBuilds";
import { workspaceById } from "#/api/queries/workspaces";
import {
	PrebuildsSystemUserID,
	type Workspace,
	type WorkspaceBuild,
} from "#/api/typesGenerated";

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
