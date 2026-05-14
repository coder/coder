import { isApiErrorResponse } from "#/api/errors";
import { ChatAttachmentMediaTypes } from "#/api/typesGenerated";

const undisplayableAttachmentDetail = "File exists but could not be displayed.";

export type AttachmentFailure =
	| { kind: "expired" }
	| { kind: "failed"; detail?: string };

export const getChatFileURL = (fileId: string) =>
	`/api/experimental/chats/files/${encodeURIComponent(fileId)}`;

export const isAbortError = (error: unknown): error is Error =>
	error instanceof Error && error.name === "AbortError";

export const attachmentFailureFromError = (
	error: unknown,
): AttachmentFailure => ({
	kind: "failed",
	detail: error instanceof Error ? error.message : undefined,
});

/**
 * Converts a chat attachment HTTP response into an availability classification.
 */
export async function classifyAttachmentFailureResponse(
	response: Response,
): Promise<AttachmentFailure> {
	if (response.status === 404) {
		return { kind: "expired" };
	}
	if (response.ok) {
		return { kind: "failed", detail: undisplayableAttachmentDetail };
	}

	// Prefer the API's structured error message (coderd returns
	// codersdk.Response { message, detail }). Fall back to the status
	// line when the body isn't JSON, for example when a proxy inserted
	// an HTML page, so the tooltip still surfaces something concrete.
	let detail = response.statusText
		? `${response.status} ${response.statusText}`
		: `HTTP ${response.status}`;
	try {
		const body: unknown = await response.json();
		if (isApiErrorResponse(body) && body.message.trim()) {
			detail = body.message;
		}
	} catch {
		// Body wasn't JSON; stick with the status line.
	}
	return { kind: "failed", detail };
}

/**
 * Performs a follow-up fetch for an attachment that failed to render locally.
 */
export async function probeAttachmentFailure(
	src: string,
	signal?: AbortSignal,
): Promise<AttachmentFailure> {
	const response = await fetch(src, { signal });
	return classifyAttachmentFailureResponse(response);
}

// Filename extensions to list in the file-picker's `accept` attribute
// alongside the MIME types. Browsers and operating systems do not always
// map these extensions to a registered MIME type (Markdown is the common
// offender), so including the extensions keeps the corresponding files
// selectable. The server still classifies uploads by byte content.
const chatAttachmentExtraExtensions = [
	".md",
	".markdown",
	".csv",
	".json",
	".txt",
] as const;

/**
 * `accept` attribute for the chat-attachment file input. Mirrors
 * codersdk.AllChatAttachmentMediaTypes so the OS file picker advertises
 * exactly what the server will accept.
 */
export const chatAttachmentAcceptAttribute = [
	...ChatAttachmentMediaTypes,
	...chatAttachmentExtraExtensions,
].join(",");

/**
 * Returns true for files whose declared MIME type is on the server
 * allowlist. Files whose type is unknown, either as an empty string or
 * as application/octet-stream, also pass so dropped or pasted files can
 * still reach the server, which remains the authority on attachment
 * bytes.
 */
export const isChatAttachmentFile = (file: File): boolean => {
	if (!file.type || file.type === "application/octet-stream") {
		return true;
	}
	return ChatAttachmentMediaTypes.some((mediaType) => mediaType === file.type);
};
