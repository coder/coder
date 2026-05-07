import { isAxiosError } from "axios";
import type { WorkspaceBuild } from "#/api/typesGenerated";

// The hard-coded UUID of the Coder prebuilds system user. Prebuilt
// workspaces are owned by this user until claim. Build #1 of a
// claimed workspace is permanently attributed to this user as the
// initiator, which is how we recognize prebuild claims after the
// fact.
//
// This UUID is stable and lives in coderd/database/constants.go on
// the backend. If it ever changes, both sides must move in lockstep.
const PREBUILDS_SYSTEM_USER_ID = "c42fdf75-3097-471c-8c33-fb52454d81c0";

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
	if (build1.initiator_id !== PREBUILDS_SYSTEM_USER_ID) {
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
 * Detects whether an error indicates a missing or deleted workspace.
 *
 * The Coder backend returns 404 when a workspace does not exist or
 * the user lacks access (to avoid leaking resource existence), and
 * 410 Gone when a workspace has been soft-deleted. Both cases mean
 * the workspace is unavailable for deletion.
 *
 * In the archive-and-delete flow this is acceptable: the workspace
 * ID comes from the chat's own metadata, so if the user can see the
 * chat they almost certainly had access to the workspace. Treating
 * an auth 404 as "already gone" is a safe degradation because the
 * user cannot delete a workspace they lack access to anyway.
 */
export function isWorkspaceNotFound(error: unknown): boolean {
	const status = isAxiosError(error) ? error.response?.status : undefined;
	return status === 404 || status === 410;
}

/**
 * Archives a chat and then deletes its associated workspace.
 * If the workspace is already gone (404 or 410), the delete step is
 * treated as a no-op so the archive still succeeds.
 */
export async function archiveChatAndDeleteWorkspace(
	chatId: string,
	workspaceId: string,
	doArchive: (chatId: string) => Promise<unknown>,
	doDelete: (workspaceId: string) => Promise<unknown>,
): Promise<{ chatId: string; workspaceId: string }> {
	await doArchive(chatId);
	try {
		await doDelete(workspaceId);
	} catch (error) {
		if (!isWorkspaceNotFound(error)) {
			throw error;
		}
	}
	return { chatId, workspaceId };
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
export async function resolveArchiveAndDeleteAction(
	fetchWorkspace: () => Promise<{ created_at: string }>,
	fetchBuilds: () => Promise<
		readonly Pick<
			WorkspaceBuild,
			"build_number" | "initiator_id" | "created_at"
		>[]
	>,
	getChatCreatedAt: () => string | undefined,
): Promise<"proceed" | "confirm" | "archive-only"> {
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
