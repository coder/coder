// Workspace files attached on the new-chat page when the chat has no
// running workspace yet. The chat page picks them up and uploads them
// once the agent has started a workspace. File objects cannot survive a
// reload, so an in-memory handoff is all that is needed.
const parkedUploadsByChatId = new Map<string, readonly File[]>();

export const parkWorkspaceUploads = (
	chatId: string,
	files: readonly File[],
): void => {
	if (files.length > 0) {
		parkedUploadsByChatId.set(chatId, files);
	}
};

export const getParkedWorkspaceUploads = (chatId: string): readonly File[] =>
	parkedUploadsByChatId.get(chatId) ?? [];

export const clearParkedWorkspaceUploads = (chatId: string): void => {
	parkedUploadsByChatId.delete(chatId);
};
