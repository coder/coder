/**
 * Roughly 1-2 lines of typical code at normal terminal width.
 * Short enough to fit in attachment previews without excessive wrapping.
 */
export const TEXT_ATTACHMENT_PREVIEW_LENGTH = 150;

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
