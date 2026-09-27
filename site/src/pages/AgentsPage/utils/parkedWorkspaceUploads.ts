import type { WorkspaceFileUpload } from "../hooks/useWorkspaceFileUploads";

type ParkedWorkspaceUpload = {
	file: File;
	// Set when the last upload attempt failed. The file then waits for
	// the user to retry or remove it instead of uploading on its own.
	failedUpload?: WorkspaceFileUpload;
};

// Workspace files waiting for a chat's workspace to start, keyed by chat
// ID. Entries outlive the components that read them but not a reload,
// which File objects cannot survive, so the page asks before unloading
// while any file is parked.
const parkedUploadsByChatId = new Map<
	string,
	readonly ParkedWorkspaceUpload[]
>();

const warnBeforeUnload = (event: BeforeUnloadEvent) => {
	event.preventDefault();
	// Chrome before 119 ignores preventDefault here and needs returnValue.
	event.returnValue = true;
};

const setParkedWorkspaceUploads = (
	chatId: string,
	uploads: readonly ParkedWorkspaceUpload[],
) => {
	if (uploads.length > 0) {
		parkedUploadsByChatId.set(chatId, uploads);
	} else {
		parkedUploadsByChatId.delete(chatId);
	}
	if (parkedUploadsByChatId.size > 0) {
		window.addEventListener("beforeunload", warnBeforeUnload);
	} else {
		window.removeEventListener("beforeunload", warnBeforeUnload);
	}
};

const replaceParkedWorkspaceUpload = (
	chatId: string,
	next: ParkedWorkspaceUpload,
) => {
	setParkedWorkspaceUploads(
		chatId,
		getParkedWorkspaceUploads(chatId).map((upload) =>
			upload.file === next.file ? next : upload,
		),
	);
};

export const getParkedWorkspaceUploads = (
	chatId: string,
): readonly ParkedWorkspaceUpload[] => parkedUploadsByChatId.get(chatId) ?? [];

export const parkWorkspaceUploads = (
	chatId: string,
	files: readonly File[],
): void => {
	setParkedWorkspaceUploads(chatId, [
		...getParkedWorkspaceUploads(chatId),
		...files.map((file) => ({ file })),
	]);
};

export const unparkWorkspaceUploads = (
	chatId: string,
	files: readonly File[],
): void => {
	setParkedWorkspaceUploads(
		chatId,
		getParkedWorkspaceUploads(chatId).filter(
			(upload) => !files.includes(upload.file),
		),
	);
};

export const markParkedWorkspaceUploadFailed = (
	chatId: string,
	failedUpload: WorkspaceFileUpload,
): void => {
	replaceParkedWorkspaceUpload(chatId, {
		file: failedUpload.file,
		failedUpload,
	});
};

/**
 * Clears a failed file's failure so it parks again. Returns false when the
 * file is not failed, so a repeated retry does nothing.
 */
export const clearParkedWorkspaceUploadFailure = (
	chatId: string,
	file: File,
): boolean => {
	const isFailed = getParkedWorkspaceUploads(chatId).some(
		(upload) => upload.file === file && upload.failedUpload !== undefined,
	);
	if (isFailed) {
		replaceParkedWorkspaceUpload(chatId, { file });
	}
	return isFailed;
};
