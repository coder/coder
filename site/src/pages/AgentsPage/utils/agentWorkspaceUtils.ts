/**
 * Determines whether a workspace was auto-created by a chat.
 * Workspaces created at or after the chat's creation time are
 * considered auto-created (the chat provisioned them). Pre-existing
 * workspaces that were manually associated need a confirmation
 * dialog before deletion.
 */
export function isWorkspaceAutoCreated(
	workspaceCreatedAt: string,
	chatCreatedAt: string,
): boolean {
	return new Date(workspaceCreatedAt) >= new Date(chatCreatedAt);
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
 * to compare its creation time against the chat's. Auto-created
 * workspaces (provisioned by the chat) skip the confirmation
 * dialog; pre-existing workspaces require the user to type the
 * workspace name.
 *
 * @param fetchWorkspace - Retrieves the workspace (e.g. via
 *   `queryClient.fetchQuery`). The result must include
 *   `created_at`.
 * @param getChatCreatedAt - Returns the chat's `created_at`
 *   timestamp, or `undefined` if the chat is not in the cache.
 * @returns `"proceed"` to skip the dialog, `"confirm"` to show it.
 */
export async function resolveArchiveAndDeleteAction(
	fetchWorkspace: () => Promise<{ created_at: string }>,
	getChatCreatedAt: () => string | undefined,
): Promise<"proceed" | "confirm"> {
	const workspace = await fetchWorkspace();
	const chatCreatedAt = getChatCreatedAt();
	if (
		chatCreatedAt &&
		isWorkspaceAutoCreated(workspace.created_at, chatCreatedAt)
	) {
		return "proceed";
	}
	return "confirm";
}
