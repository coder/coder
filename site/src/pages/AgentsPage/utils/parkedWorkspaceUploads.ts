// Workspace files waiting for a chat's workspace to start, keyed by chat
// ID. Entries outlive the components that read them but not a reload,
// which File objects cannot survive, so the page asks before unloading
// while any file is parked.
const parkedUploadsByChatId = new Map<string, readonly File[]>();

const warnBeforeUnload = (event: BeforeUnloadEvent) => {
	event.preventDefault();
	// Chrome before 119 ignores preventDefault here and needs returnValue.
	event.returnValue = true;
};

const setParkedWorkspaceUploads = (chatId: string, files: readonly File[]) => {
	if (files.length > 0) {
		parkedUploadsByChatId.set(chatId, files);
	} else {
		parkedUploadsByChatId.delete(chatId);
	}
	if (parkedUploadsByChatId.size > 0) {
		window.addEventListener("beforeunload", warnBeforeUnload);
	} else {
		window.removeEventListener("beforeunload", warnBeforeUnload);
	}
};

export const getParkedWorkspaceUploads = (chatId: string): readonly File[] =>
	parkedUploadsByChatId.get(chatId) ?? [];

export const parkWorkspaceUploads = (
	chatId: string,
	files: readonly File[],
): void => {
	setParkedWorkspaceUploads(chatId, [
		...getParkedWorkspaceUploads(chatId),
		...files,
	]);
};

export const unparkWorkspaceUploads = (
	chatId: string,
	files: readonly File[],
): void => {
	setParkedWorkspaceUploads(
		chatId,
		getParkedWorkspaceUploads(chatId).filter((file) => !files.includes(file)),
	);
};
