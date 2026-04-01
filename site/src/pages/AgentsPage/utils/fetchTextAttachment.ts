/**
 * Roughly 1-2 lines of typical code at normal terminal width.
 * Short enough to fit in attachment previews without excessive wrapping.
 */
const TEXT_ATTACHMENT_PREVIEW_LENGTH = 150;

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
): Promise<string> {
	const response = await fetch(`/api/experimental/chats/files/${fileId}`, {
		signal,
	});
	if (!response.ok) {
		throw new Error("Failed to fetch file");
	}
	return response.text();
}
