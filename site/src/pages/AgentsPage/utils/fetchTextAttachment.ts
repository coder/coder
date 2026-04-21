import { isApiErrorResponse } from "#/api/errors";

/**
 * Roughly 1-2 lines of typical code at normal terminal width.
 * Short enough to fit in attachment previews without excessive wrapping.
 */
const TEXT_ATTACHMENT_PREVIEW_LENGTH = 150;
const undisplayableFileDetail = "File exists but could not be displayed.";

export type AttachmentFailure =
	| { kind: "expired" }
	| { kind: "failed"; detail?: string };

type TextAttachmentLoadResult =
	| { kind: "loaded"; content: string }
	| AttachmentFailure;

/**
 * Converts a chat attachment HTTP response into an availability classification.
 */
async function classifyAttachmentFailureResponse(
	response: Response,
): Promise<AttachmentFailure> {
	if (response.status === 404) {
		return { kind: "expired" };
	}
	if (response.ok) {
		return { kind: "failed", detail: undisplayableFileDetail };
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

export function formatTextAttachmentPreview(
	text: string,
	maxLength = TEXT_ATTACHMENT_PREVIEW_LENGTH,
): string {
	// Truncate before normalizing to keep cost bounded for large files.
	const truncated = text.slice(0, maxLength * 4);
	const normalized = truncated.replace(/\s+/g, " ").trim();
	const preview = Array.from(normalized).slice(0, maxLength).join("");
	return preview || "Pasted text";
}

/**
 * Decodes inline text attachment data encoded as base64 UTF-8.
 */
export function decodeInlineTextAttachment(content: string): string {
	try {
		const decoded = atob(content);
		const bytes = Uint8Array.from(decoded, (char) => char.charCodeAt(0));
		return new TextDecoder().decode(bytes);
	} catch (err) {
		console.warn("Failed to decode inline text attachment:", err);
		return content;
	}
}

/**
 * Fetches the text content of a chat file attachment by its ID.
 */
export async function fetchTextAttachmentContent(
	fileId: string,
	signal?: AbortSignal,
): Promise<TextAttachmentLoadResult> {
	const response = await fetch(`/api/experimental/chats/files/${fileId}`, {
		signal,
	});
	if (response.ok) {
		return { kind: "loaded", content: await response.text() };
	}
	return classifyAttachmentFailureResponse(response);
}
