import { getErrorMessage, isApiError, isApiErrorResponse } from "#/api/errors";
import {
	type ChatFilePurpose,
	MaxChatFileSizeBytes,
	MaxChatWorkspaceFileSizeBytes,
} from "#/api/typesGenerated";

/** The upload endpoint's size limit in bytes; provider image budgets are a separate check. */
export const maxAgentAttachmentBytes = (purpose: ChatFilePurpose): number =>
	purpose === "workspace"
		? MaxChatWorkspaceFileSizeBytes
		: MaxChatFileSizeBytes;

export const formatAgentAttachmentTooLargeError = (
	fileSize: number,
	purpose: ChatFilePurpose = "prompt",
): string =>
	`File too large (${(fileSize / 1024 / 1024).toFixed(1)} MiB). Maximum is ${maxAgentAttachmentBytes(purpose) / 1024 / 1024} MiB.`;

// Unlike getErrorDetail, never fall back to the developer console hint:
// upload errors render on compact chips next to the file.
const getUploadErrorDetail = (error: unknown): string | undefined => {
	if (isApiError(error)) {
		return error.response.data.detail;
	}
	if (isApiErrorResponse(error)) {
		return error.detail;
	}
	return undefined;
};

export const formatAgentAttachmentUploadError = (error: unknown): string => {
	const message = getErrorMessage(error, "Upload failed").trim();
	const detail = getUploadErrorDetail(error)?.trim();
	if (!detail) {
		return message;
	}
	const separator = /[.!?]$/.test(message) ? " " : ". ";
	return `${message}${separator}${detail}`;
};

export const readAgentAttachmentText = (file: File): Promise<string> => {
	if (typeof file.text === "function") {
		return file.text();
	}
	return new Promise((resolve, reject) => {
		const reader = new FileReader();
		reader.onerror = () =>
			reject(reader.error ?? new Error("Failed to read file content."));
		reader.onload = () => {
			if (typeof reader.result === "string") {
				resolve(reader.result);
				return;
			}
			reject(new Error("Failed to read file content."));
		};
		reader.readAsText(file);
	});
};
