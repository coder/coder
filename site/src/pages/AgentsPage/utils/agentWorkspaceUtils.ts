import { isAxiosError } from "axios";

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
 * immediately or require user confirmation. Checks whether the
 * workspace still exists and uses the `workspace_auto_created`
 * boolean from the chat to decide.
 *
 * @param fetchWorkspace - Retrieves the workspace (e.g. via
 *   `queryClient.fetchQuery`). Used only to verify the workspace
 *   still exists.
 * @param isAutoCreated - The chat's `workspace_auto_created` field.
 *   When true the confirmation dialog is skipped.
 * @returns `"proceed"` to skip the dialog, `"archive-only"` to archive
 *   without deleting because the workspace is already gone, or
 *   `"confirm"` to show the dialog.
 */
export async function resolveArchiveAndDeleteAction(
	fetchWorkspace: () => Promise<unknown>,
	isAutoCreated: boolean,
): Promise<"proceed" | "confirm" | "archive-only"> {
	try {
		await fetchWorkspace();
	} catch (error) {
		if (isWorkspaceNotFound(error)) {
			return "archive-only";
		}
		throw error;
	}
	if (isAutoCreated) {
		return "proceed";
	}
	return "confirm";
}
