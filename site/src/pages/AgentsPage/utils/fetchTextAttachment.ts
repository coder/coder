import {
	type AttachmentFailure,
	classifyAttachmentFailureResponse,
	getChatFileURL,
} from "./chatAttachments";

/**
 * Roughly 1-2 lines of typical code at normal terminal width.
 * Short enough to fit in attachment previews without excessive wrapping.
 */
const TEXT_ATTACHMENT_PREVIEW_LENGTH = 150;

type TextAttachmentLoadResult =
	| { kind: "loaded"; content: string }
	| AttachmentFailure;

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
 * Encodes UTF-8 text as base64. Inverse of decodeInlineTextAttachment.
 */
export function encodeInlineTextAttachment(text: string): string {
	const bytes = new TextEncoder().encode(text);
	return btoa(String.fromCharCode(...bytes));
}

export function getTextAttachmentErrorMessage(error: unknown): string | null {
	if (
		typeof error === "object" &&
		error !== null &&
		"name" in error &&
		error.name === "AbortError"
	) {
		return null;
	}

	return "Couldn't load preview. Select again to retry.";
}

/**
 * Fetches the text content of a chat file attachment by its ID.
 */
export async function fetchTextAttachmentContent(
	fileId: string,
	signal?: AbortSignal,
): Promise<TextAttachmentLoadResult> {
	const response = await fetch(getChatFileURL(fileId), { signal });
	if (response.ok) {
		return { kind: "loaded", content: await response.text() };
	}
	return classifyAttachmentFailureResponse(response);
}
