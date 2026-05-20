import { isApiErrorResponse } from "#/api/errors";
import {
	ChatAttachmentMediaTypes,
	type ChatInputPart,
	type ChatMessagePart,
	type ChatWorkspaceFileReferencePart,
} from "#/api/typesGenerated";

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

/**
 * Returns true only for files whose MIME type positively matches the
 * chat attachment allowlist. Unknown or octet-stream files return
 * false so callers can route them to the workspace upload flow.
 */
export const isStrictChatAttachmentFile = (file: File): boolean =>
	Boolean(file.type) &&
	file.type !== "application/octet-stream" &&
	ChatAttachmentMediaTypes.some((mediaType) => mediaType === file.type);

// Matches characters that commonly cause trouble downstream: bracketing
// punctuation, quotes, shell or URL or path metacharacters, path
// separators, any whitespace, and control characters. ASCII alphanumerics,
// `.`, `-`, `_`, and all other Unicode letters and symbols (CJK, emoji,
// accented Latin) are preserved so localized filenames remain readable.
const unsafeChatFileNameChars = /[()[\]{}<>'"`;,:*?|&#$\\/\s\p{Cc}]/gu;

/**
 * Replaces characters that commonly cause trouble downstream (shells,
 * LLM prompts, audit logs, path interpolation) with underscores. Keeps
 * dots, dashes, underscores, ASCII alphanumerics, and non-ASCII letters
 * so localized names remain readable. The server still applies its own
 * normalization (control-char strip plus 255-byte truncate) on top of this.
 *
 * If the sanitized name is empty after trimming leading or trailing `_`,
 * `.`, or whitespace, falls back to `"file"` so the server's
 * "filename required" contract still holds.
 */
export const sanitizeChatFileName = (name: string): string => {
	const replaced = name.replace(unsafeChatFileNameChars, "_");
	// Collapse runs of underscores introduced by replacement into a single
	// underscore so `foo (final).pdf` becomes `foo_final_.pdf` rather than
	// `foo__final_.pdf`. Pre-existing `__` in the original name is also
	// collapsed; acceptable tradeoff for tidier names.
	const collapsed = replaced.replace(/_+/g, "_");
	const trimmed = collapsed.replace(/^[_.\s]+|[_.\s]+$/g, "");
	return trimmed === "" ? "file" : trimmed;
};

type workspaceFileReferenceInput = {
	path: string;
	name: string;
	size: number;
	mediaType: string;
};

export const isWorkspaceFileReferencePart = (
	part: ChatMessagePart,
): part is ChatWorkspaceFileReferencePart =>
	part.type === "workspace-file-reference";

export const workspaceFileReferencePart = (
	upload: workspaceFileReferenceInput,
): ChatInputPart => ({
	type: "workspace-file-reference",
	workspace_file_path: upload.path,
	workspace_file_name: upload.name,
	workspace_file_size: upload.size,
	workspace_file_media_type: upload.mediaType,
});

/**
 * Returns a new File whose `name` is sanitized via `sanitizeChatFileName`.
 * If the sanitized name is identical to the original, returns the input
 * File unchanged to preserve referential equality. The chat UI keys
 * preview-URL, upload-state, and text-content Maps on the File object,
 * so identity must be stable for already-safe names.
 */
export const renameChatFileForUpload = (file: File): File => {
	const sanitized = sanitizeChatFileName(file.name);
	if (sanitized === file.name) {
		return file;
	}
	return new File([file], sanitized, {
		type: file.type,
		lastModified: file.lastModified,
	});
};
